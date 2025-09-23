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

package managers

import (
	"context"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
	"github.com/cappyzawa/score-orchestrator/internal/config"
	"github.com/cappyzawa/score-orchestrator/internal/endpoint"
	"github.com/cappyzawa/score-orchestrator/internal/status"
)

// Mock implementations
type mockConfigLoader struct {
	loadConfigFunc func(ctx context.Context) (*scorev1b1.OrchestratorConfig, error)
	watchFunc      func(ctx context.Context) (<-chan config.ConfigEvent, error)
	closeFunc      func() error
}

func (m *mockConfigLoader) LoadConfig(ctx context.Context) (*scorev1b1.OrchestratorConfig, error) {
	if m.loadConfigFunc != nil {
		return m.loadConfigFunc(ctx)
	}
	return nil, nil
}

func (m *mockConfigLoader) Watch(ctx context.Context) (<-chan config.ConfigEvent, error) {
	if m.watchFunc != nil {
		return m.watchFunc(ctx)
	}
	return nil, nil
}

func (m *mockConfigLoader) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

type mockEventRecorder struct {
	record.EventRecorder
	events []string
}

func (m *mockEventRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
	m.events = append(m.events, reason)
}

var _ = Describe("PlanManager", func() {
	var (
		scheme   *runtime.Scheme
		workload *scorev1b1.Workload
		claims   []scorev1b1.ResourceClaim
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(scorev1b1.AddToScheme(scheme)).ToNot(HaveOccurred())

		workload = &scorev1b1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workload",
				Namespace: "test-ns",
			},
			Spec: scorev1b1.WorkloadSpec{
				Containers: map[string]scorev1b1.ContainerSpec{
					"main": {
						Image: "nginx:latest",
					},
				},
			},
		}

		claims = []scorev1b1.ResourceClaim{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-claim",
					Namespace: "test-ns",
				},
				Spec: scorev1b1.ResourceClaimSpec{
					Key:  "database",
					Type: "postgres",
				},
				Status: scorev1b1.ResourceClaimStatus{
					Phase:            "Bound",
					OutputsAvailable: true,
					Outputs: &scorev1b1.ResourceClaimOutputs{
						URI: stringPtr("postgres://localhost:5432/test"),
					},
				},
			},
		}
	})

	Describe("EnsurePlan", func() {
		Context("when claims are ready", func() {
			It("should create plan", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
				mockRecorder := &mockEventRecorder{}

				// Mock config loading
				testConfig := &scorev1b1.OrchestratorConfig{
					Spec: scorev1b1.OrchestratorConfigSpec{
						Profiles: []scorev1b1.ProfileSpec{
							{
								Name: "test-profile",
								Backends: []scorev1b1.BackendSpec{
									{
										BackendId:    "test-backend",
										RuntimeClass: "kubernetes",
										Priority:     100,
										Template: scorev1b1.TemplateSpec{
											Kind:   "manifests",
											Ref:    "test-template:latest",
											Values: nil, // Use nil for MVP test
										},
									},
								},
							},
						},
						Defaults: scorev1b1.DefaultsSpec{
							Profile: "test-profile",
						},
					},
				}

				mockConfigLoader := &mockConfigLoader{
					loadConfigFunc: func(ctx context.Context) (*scorev1b1.OrchestratorConfig, error) {
						return testConfig, nil
					},
				}

				// Create a status manager for the test
				statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
				pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

				agg := status.ClaimAggregation{
					Ready:   true,
					Message: "All claims are ready",
				}

				err := pm.EnsurePlan(context.Background(), workload, claims, agg)
				Expect(err).ToNot(HaveOccurred())

				// Verify WorkloadPlan was created
				planList := &scorev1b1.WorkloadPlanList{}
				err = fakeClient.List(context.Background(), planList, client.InNamespace("test-ns"))
				Expect(err).ToNot(HaveOccurred())
				Expect(planList.Items).To(HaveLen(1))

				plan := planList.Items[0]
				Expect(plan.Name).To(Equal("test-workload"))
				Expect(plan.Namespace).To(Equal("test-ns"))
				Expect(plan.Spec.RuntimeClass).To(Equal("kubernetes"))

				// Verify event was recorded
				Expect(mockRecorder.events).To(ContainElement(EventReasonPlanCreated))
			})
		})

		Context("when claims are not ready", func() {
			It("should skip plan creation", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				mockConfigLoader := &mockConfigLoader{}
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
				mockRecorder := &mockEventRecorder{}

				// Create a status manager for the test
				statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
				pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

				agg := status.ClaimAggregation{
					Ready:   false,
					Message: "Claims are not ready",
				}

				err := pm.EnsurePlan(context.Background(), workload, claims, agg)
				Expect(err).ToNot(HaveOccurred())

				// Verify no WorkloadPlan was created
				planList := &scorev1b1.WorkloadPlanList{}
				err = fakeClient.List(context.Background(), planList, client.InNamespace("test-ns"))
				Expect(err).ToNot(HaveOccurred())
				Expect(planList.Items).To(BeEmpty())

				// Verify no events were recorded
				Expect(mockRecorder.events).To(BeEmpty())
			})
		})

		Context("when backend selection fails", func() {
			It("should handle error", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
				mockRecorder := &mockEventRecorder{}

				// Mock config loading failure
				mockConfigLoader := &mockConfigLoader{
					loadConfigFunc: func(ctx context.Context) (*scorev1b1.OrchestratorConfig, error) {
						return nil, fmt.Errorf("config loading failed")
					},
				}

				// Create a status manager for the test
				statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
				pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

				agg := status.ClaimAggregation{
					Ready:   true,
					Message: "All claims are ready",
				}

				err := pm.EnsurePlan(context.Background(), workload, claims, agg)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load orchestrator config"))

				// Verify condition was set on workload
				runtimeCondition := conditions.GetCondition(workload.Status.Conditions, conditions.ConditionRuntimeReady)
				Expect(runtimeCondition).ToNot(BeNil())
				Expect(runtimeCondition.Status).To(Equal(metav1.ConditionFalse))
				Expect(runtimeCondition.Reason).To(Equal(conditions.ReasonRuntimeSelecting))
			})
		})

		Context("when placeholders are unresolved", func() {
			It("should skip plan creation and set ProjectionError", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
				mockRecorder := &mockEventRecorder{}

				// Mock config loading for successful backend selection
				testConfig := &scorev1b1.OrchestratorConfig{
					Spec: scorev1b1.OrchestratorConfigSpec{
						Profiles: []scorev1b1.ProfileSpec{
							{
								Name: "test-profile",
								Backends: []scorev1b1.BackendSpec{
									{
										BackendId:    "test-backend",
										RuntimeClass: "kubernetes",
										Priority:     100,
										Template: scorev1b1.TemplateSpec{
											Kind: "manifests",
											Ref:  "test-template:latest",
										},
									},
								},
							},
						},
						Defaults: scorev1b1.DefaultsSpec{
							Profile: "test-profile",
						},
					},
				}

				mockConfigLoader := &mockConfigLoader{
					loadConfigFunc: func(ctx context.Context) (*scorev1b1.OrchestratorConfig, error) {
						return testConfig, nil
					},
				}

				// Create a status manager for the test
				statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
				pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

				// Create workload with unresolved placeholders
				workloadWithPlaceholders := &scorev1b1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workload",
						Namespace: "test-ns",
					},
					Spec: scorev1b1.WorkloadSpec{
						Containers: map[string]scorev1b1.ContainerSpec{
							"main": {
								Image: "nginx:latest",
								Variables: map[string]string{
									"DB_HOST": "${resources.db.host}",
								},
							},
						},
					},
				}

				// Claims are ready but outputs contain unresolved placeholders
				claimsWithUnresolvedOutputs := []scorev1b1.ResourceClaim{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-claim",
							Namespace: "test-ns",
						},
						Spec: scorev1b1.ResourceClaimSpec{
							Key:  "db",
							Type: "postgres",
						},
						Status: scorev1b1.ResourceClaimStatus{
							Phase:            "Bound",
							OutputsAvailable: false, // Outputs not available, causing unresolved placeholders
						},
					},
				}

				agg := status.ClaimAggregation{
					Ready:   true,
					Message: "All claims are ready",
				}

				// Act
				err := pm.EnsurePlan(context.Background(), workloadWithPlaceholders, claimsWithUnresolvedOutputs, agg)

				// Assert: Should return error due to unresolved placeholders
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to resolve placeholders"))

				// Verify no WorkloadPlan was created
				planList := &scorev1b1.WorkloadPlanList{}
				err = fakeClient.List(context.Background(), planList, client.InNamespace("test-ns"))
				Expect(err).ToNot(HaveOccurred())
				Expect(planList.Items).To(BeEmpty())

				// Verify error event was recorded (either ProjectionError or PlanError)
				Expect(mockRecorder.events).To(HaveLen(1))
				Expect(mockRecorder.events[0]).To(BeElementOf(EventReasonProjectionError, EventReasonPlanError))
			})
		})
	})

	Describe("GetPlan", func() {
		var (
			testWorkload *scorev1b1.Workload
			existingPlan *scorev1b1.WorkloadPlan
		)

		BeforeEach(func() {
			testWorkload = &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
			}

			existingPlan = &scorev1b1.WorkloadPlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
					Labels: map[string]string{
						"score.dev/workload": "test-workload",
					},
				},
				Spec: scorev1b1.WorkloadPlanSpec{
					WorkloadRef: scorev1b1.WorkloadPlanWorkloadRef{
						Name:      "test-workload",
						Namespace: "test-ns",
					},
					RuntimeClass: "kubernetes",
				},
			}
		})

		Context("when plan exists", func() {
			It("should return existing plan", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingPlan).Build()
				mockConfigLoader := &mockConfigLoader{}
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
				mockRecorder := &mockEventRecorder{}

				// Create a status manager for the test
				statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
				pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

				plan, err := pm.GetPlan(context.Background(), testWorkload)
				Expect(err).ToNot(HaveOccurred())
				Expect(plan).ToNot(BeNil())
				Expect(plan.Name).To(Equal("test-workload"))
				Expect(plan.Spec.RuntimeClass).To(Equal("kubernetes"))
			})
		})

		Context("when plan doesn't exist", func() {
			It("should return NotFound error", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				mockConfigLoader := &mockConfigLoader{}
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
				mockRecorder := &mockEventRecorder{}

				// Create a status manager for the test
				statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
				pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

				plan, err := pm.GetPlan(context.Background(), testWorkload)
				Expect(err).To(HaveOccurred())
				Expect(errors.IsNotFound(err)).To(BeTrue())
				Expect(plan).To(BeNil())
			})
		})
	})

	Describe("SelectBackend", func() {
		var (
			testWorkload *scorev1b1.Workload
		)

		BeforeEach(func() {
			testWorkload = &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"main": {
							Image: "nginx:latest",
						},
					},
				},
			}
		})

		Context("when config loading succeeds", func() {
			It("should return selected backend", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
				mockRecorder := &mockEventRecorder{}

				testConfig := &scorev1b1.OrchestratorConfig{
					Spec: scorev1b1.OrchestratorConfigSpec{
						Profiles: []scorev1b1.ProfileSpec{
							{
								Name: "test-profile",
								Backends: []scorev1b1.BackendSpec{
									{
										BackendId:    "test-backend",
										RuntimeClass: "kubernetes",
										Priority:     100,
										Template: scorev1b1.TemplateSpec{
											Kind:   "manifests",
											Ref:    "test-template:latest",
											Values: nil, // Use nil for MVP test
										},
									},
								},
							},
						},
						Defaults: scorev1b1.DefaultsSpec{
							Profile: "test-profile",
						},
					},
				}

				mockConfigLoader := &mockConfigLoader{
					loadConfigFunc: func(ctx context.Context) (*scorev1b1.OrchestratorConfig, error) {
						return testConfig, nil
					},
				}

				// Create a status manager for the test
				statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
				pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

				selectedBackend, err := pm.SelectBackend(context.Background(), testWorkload)
				Expect(err).ToNot(HaveOccurred())
				Expect(selectedBackend).ToNot(BeNil())
				Expect(selectedBackend.BackendID).To(Equal("test-backend"))
				Expect(selectedBackend.RuntimeClass).To(Equal("kubernetes"))
			})
		})

		Context("when config loading fails", func() {
			It("should handle error", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)
				mockRecorder := &mockEventRecorder{}

				mockConfigLoader := &mockConfigLoader{
					loadConfigFunc: func(ctx context.Context) (*scorev1b1.OrchestratorConfig, error) {
						return nil, fmt.Errorf("config loading failed")
					},
				}

				// Create a status manager for the test
				statusManager := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)
				pm := NewPlanManager(fakeClient, scheme, mockRecorder, mockConfigLoader, endpointDeriver, statusManager)

				selectedBackend, err := pm.SelectBackend(context.Background(), testWorkload)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to load orchestrator config"))
				Expect(selectedBackend).To(BeNil())
			})
		})
	})
})
