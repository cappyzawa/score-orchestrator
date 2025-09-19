/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package provisioner

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/config"
)

// mockConfigLoader implements config.Loader for testing
type mockConfigLoader struct {
	config *scorev1b1.OrchestratorConfig
	err    error
}

func (m *mockConfigLoader) Load(ctx context.Context) (*scorev1b1.OrchestratorConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.config, nil
}

func (m *mockConfigLoader) Watch(ctx context.Context) (<-chan config.ConfigEvent, error) {
	return nil, nil
}

func (m *mockConfigLoader) Close() error {
	return nil
}

// mockEventRecorder implements record.EventRecorder for testing
type mockEventRecorder struct {
	events []string
}

func (m *mockEventRecorder) Event(object runtime.Object, eventtype, reason, message string) {
	m.events = append(m.events, reason)
}

func (m *mockEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	m.events = append(m.events, reason)
}

func (m *mockEventRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	m.events = append(m.events, reason)
}

var _ = Describe("ProvisionerReconciler", func() {
	var (
		reconciler     *ProvisionerReconciler
		fakeClient     client.Client
		configLoader   *mockConfigLoader
		eventRecorder  *mockEventRecorder
		scheme         *runtime.Scheme
		ctx            context.Context
		claim          *scorev1b1.ResourceClaim
		namespacedName types.NamespacedName
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(scorev1b1.AddToScheme(scheme)).To(Succeed())

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		configLoader = &mockConfigLoader{}
		eventRecorder = &mockEventRecorder{}

		reconciler = &ProvisionerReconciler{
			Client:       fakeClient,
			Scheme:       scheme,
			Recorder:     eventRecorder,
			ConfigLoader: configLoader,
		}

		ctx = context.Background()

		// Create test ResourceClaim
		claim = &scorev1b1.ResourceClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-claim",
				Namespace: "test-namespace",
			},
			Spec: scorev1b1.ResourceClaimSpec{
				WorkloadRef: scorev1b1.NamespacedName{
					Name:      "test-workload",
					Namespace: "test-namespace",
				},
				Key:  "database",
				Type: "postgres",
			},
		}

		namespacedName = types.NamespacedName{
			Name:      claim.Name,
			Namespace: claim.Namespace,
		}

		// Create the claim in the fake client
		Expect(fakeClient.Create(ctx, claim)).To(Succeed())
	})

	Context("When reconciling a ResourceClaim", func() {
		It("should handle configuration load failure", func() {
			configLoader.err = fmt.Errorf("config not found")

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(time.Minute))
			Expect(eventRecorder.events).To(ContainElement("ConfigLoadFailed"))
		})

		It("should handle missing provisioner configuration", func() {
			configLoader.config = &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Provisioners: []scorev1b1.ProvisionerSpec{
						{
							Type: "redis", // Different type
							Config: &scorev1b1.ProvisionerConfig{
								Strategy: "manifests",
							},
						},
					},
				},
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify claim status is updated to failed
			var updatedClaim scorev1b1.ResourceClaim
			Expect(fakeClient.Get(ctx, namespacedName, &updatedClaim)).To(Succeed())
			Expect(updatedClaim.Status.Phase).To(Equal(scorev1b1.ResourceClaimPhaseFailed))
			Expect(updatedClaim.Status.Reason).To(Equal("ProvisionerNotFound"))
		})

		It("should handle Helm strategy provisioning", func() {
			helmValues := &runtime.RawExtension{
				Raw: []byte(`{"auth":{"postgresPassword":"{{.secret.password}}"}}`),
			}

			configLoader.config = &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Provisioners: []scorev1b1.ProvisionerSpec{
						{
							Type: "postgres",
							Config: &scorev1b1.ProvisionerConfig{
								Strategy: "helm",
								Helm: &scorev1b1.HelmStrategy{
									Chart:      "bitnami/postgresql",
									Repository: "https://charts.bitnami.com/bitnami",
									Values:     helmValues,
								},
								Outputs: map[string]string{
									"uri": "postgresql://postgres:{{.secret.password}}@{{.service.name}}:5432/postgres",
								},
							},
						},
					},
				},
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))

			// Verify claim status is updated to claiming
			var updatedClaim scorev1b1.ResourceClaim
			Expect(fakeClient.Get(ctx, namespacedName, &updatedClaim)).To(Succeed())
			Expect(updatedClaim.Status.Phase).To(Equal(scorev1b1.ResourceClaimPhaseClaiming))
			Expect(updatedClaim.Status.Reason).To(Equal("HelmDeploymentStarted"))
		})

		It("should handle Manifests strategy provisioning", func() {
			manifestData := &runtime.RawExtension{
				Raw: []byte(`{
					"apiVersion": "apps/v1",
					"kind": "Deployment",
					"metadata": {
						"name": "{{.claimName}}-redis",
						"namespace": "{{.namespace}}"
					},
					"spec": {
						"replicas": 1,
						"selector": {"matchLabels": {"app": "{{.claimName}}-redis"}}
					}
				}`),
			}

			configLoader.config = &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Provisioners: []scorev1b1.ProvisionerSpec{
						{
							Type: "postgres",
							Config: &scorev1b1.ProvisionerConfig{
								Strategy:  "manifests",
								Manifests: []runtime.RawExtension{*manifestData},
								Outputs: map[string]string{
									"uri": "redis://{{.service.name}}:6379",
								},
							},
						},
					},
				},
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})

			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))

			// Verify claim status is updated to claiming
			var updatedClaim scorev1b1.ResourceClaim
			Expect(fakeClient.Get(ctx, namespacedName, &updatedClaim)).To(Succeed())
			Expect(updatedClaim.Status.Phase).To(Equal(scorev1b1.ResourceClaimPhaseClaiming))
			Expect(updatedClaim.Status.Reason).To(Equal("ManifestDeploymentStarted"))
		})

		It("should handle legacy provisioner configuration", func() {
			configLoader.config = &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Provisioners: []scorev1b1.ProvisionerSpec{
						{
							Type:        "postgres",
							Provisioner: "postgres-operator", // Legacy field
							// No Config field
						},
					},
				},
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify claim status is updated to pending for external controller
			var updatedClaim scorev1b1.ResourceClaim
			Expect(fakeClient.Get(ctx, namespacedName, &updatedClaim)).To(Succeed())
			Expect(updatedClaim.Status.Phase).To(Equal(scorev1b1.ResourceClaimPhasePending))
			Expect(updatedClaim.Status.Reason).To(Equal("WaitingForExternalController"))
		})

		It("should handle invalid strategy", func() {
			configLoader.config = &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Provisioners: []scorev1b1.ProvisionerSpec{
						{
							Type: "postgres",
							Config: &scorev1b1.ProvisionerConfig{
								Strategy: "invalid-strategy",
							},
						},
					},
				},
			}

			result, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespacedName})

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))

			// Verify claim status is updated to failed
			var updatedClaim scorev1b1.ResourceClaim
			Expect(fakeClient.Get(ctx, namespacedName, &updatedClaim)).To(Succeed())
			Expect(updatedClaim.Status.Phase).To(Equal(scorev1b1.ResourceClaimPhaseFailed))
			Expect(updatedClaim.Status.Reason).To(Equal("InvalidStrategy"))
		})
	})

	Context("When finding provisioner configuration", func() {
		It("should find matching provisioner by type", func() {
			config := &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Provisioners: []scorev1b1.ProvisionerSpec{
						{Type: "redis", Provisioner: "redis-operator"},
						{Type: "postgres", Provisioner: "postgres-operator"},
						{Type: "mongodb", Provisioner: "mongo-operator"},
					},
				},
			}

			result, err := reconciler.findProvisionerConfig(config, "postgres")

			Expect(err).NotTo(HaveOccurred())
			Expect(result).NotTo(BeNil())
			Expect(result.Type).To(Equal("postgres"))
			Expect(result.Provisioner).To(Equal("postgres-operator"))
		})

		It("should return error for non-existent type", func() {
			config := &scorev1b1.OrchestratorConfig{
				Spec: scorev1b1.OrchestratorConfigSpec{
					Provisioners: []scorev1b1.ProvisionerSpec{
						{Type: "redis", Provisioner: "redis-operator"},
					},
				},
			}

			result, err := reconciler.findProvisionerConfig(config, "postgres")

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeNil())
			Expect(err.Error()).To(ContainSubstring("no provisioner configuration found"))
		})
	})
})
