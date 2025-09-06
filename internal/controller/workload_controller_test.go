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

package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
	"github.com/cappyzawa/score-orchestrator/internal/meta"
	internalreconcile "github.com/cappyzawa/score-orchestrator/internal/reconcile"
)

// Helper function to create string pointer
func stringPtr(s string) *string {
	return &s
}

// Index functions for ResourceBinding and WorkloadPlan lookups (matching indexers.go)
func indexResourceBindingByWorkload(obj client.Object) []string {
	binding := obj.(*scorev1b1.ResourceBinding)
	return []string{binding.Spec.WorkloadRef.Namespace + "/" + binding.Spec.WorkloadRef.Name}
}

func indexWorkloadPlanByWorkload(obj client.Object) []string {
	plan := obj.(*scorev1b1.WorkloadPlan)
	return []string{plan.Spec.WorkloadRef.Namespace + "/" + plan.Spec.WorkloadRef.Name}
}

var _ = Describe("Workload Controller", func() {
	Context("When reconciling a resource", func() {
		var (
			testNS             *corev1.Namespace
			cancel             context.CancelFunc
			doneCh             chan struct{}
			mgr                manager.Manager
			k8sCl              client.Client
			workload           *scorev1b1.Workload
			typeNamespacedName types.NamespacedName
		)

		BeforeEach(func() {
			// 1) Create temporary namespace with unique name
			testNS = &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-",
				},
			}
			Expect(k8sClient.Create(context.Background(), testNS)).To(Succeed())
			Eventually(func() error {
				return k8sClient.Get(context.Background(), client.ObjectKey{Name: testNS.Name}, &corev1.Namespace{})
			}).Should(Succeed())

			// 2) Create Manager with NS-scoped cache and port conflict avoidance
			mgrCtx, c := context.WithCancel(context.Background())
			cancel = c
			doneCh = make(chan struct{})

			var err error
			mgr, err = ctrl.NewManager(cfg, ctrl.Options{
				Scheme: k8sClient.Scheme(),
				// Cache scoped to this test's namespace only
				Cache: cache.Options{
					DefaultNamespaces: map[string]cache.Config{
						testNS.Name: {},
					},
				},
				Metrics: server.Options{
					BindAddress: "0", // Disable metrics server
				},
				HealthProbeBindAddress: "0", // Disable health probe
				LeaderElection:         false,
			})
			Expect(err).NotTo(HaveOccurred())

			// 3) Setup indexers before starting manager
			fi := mgr.GetFieldIndexer()
			Expect(fi.IndexField(mgrCtx, &scorev1b1.ResourceBinding{}, meta.IndexResourceBindingByWorkload, indexResourceBindingByWorkload)).To(Succeed())
			Expect(fi.IndexField(mgrCtx, &scorev1b1.WorkloadPlan{}, meta.IndexWorkloadPlanByWorkload, indexWorkloadPlanByWorkload)).To(Succeed())

			// 4) Setup WorkloadReconciler with unique name for this test
			reconciler := &WorkloadReconciler{
				Client:   mgr.GetClient(),
				Scheme:   mgr.GetScheme(),
				Recorder: mgr.GetEventRecorderFor("workload-controller-test-" + testNS.Name),
			}
			// Setup controller directly with unique name to avoid conflicts between tests
			err = ctrl.NewControllerManagedBy(mgr).
				For(&scorev1b1.Workload{}).
				Owns(&scorev1b1.ResourceBinding{}).
				Owns(&scorev1b1.WorkloadPlan{}).
				Watches(&scorev1b1.ResourceBinding{}, EnqueueRequestForOwningWorkload()).
				Watches(&scorev1b1.WorkloadPlan{}, EnqueueRequestForOwningWorkload()).
				Named("workload-" + testNS.Name). // Unique name per test namespace
				Complete(reconciler)
			Expect(err).NotTo(HaveOccurred())

			// 5) Start manager and wait for cache sync
			go func() {
				defer close(doneCh)
				defer GinkgoRecover()
				Expect(mgr.Start(mgrCtx)).To(Succeed())
			}()
			// Critical: wait for cache sync before proceeding
			Expect(mgr.GetCache().WaitForCacheSync(mgrCtx)).To(BeTrue())

			k8sCl = mgr.GetClient()

			// 6) Create test workload with GenerateName
			workload = &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "test-workload-",
					Namespace:    testNS.Name,
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Image: "nginx:latest",
						},
					},
					Resources: map[string]scorev1b1.ResourceSpec{
						"db": {
							Type: "postgresql",
						},
					},
				},
			}
			Expect(k8sCl.Create(context.Background(), workload)).To(Succeed())

			// Set the typeNamespacedName after creation (since GenerateName assigns the actual name)
			typeNamespacedName = types.NamespacedName{
				Name:      workload.Name,
				Namespace: workload.Namespace,
			}
		})

		AfterEach(func() {
			// 1) Clean shutdown: Delete namespace with Foreground propagation and wait for completion
			By("Deleting test namespace with Foreground propagation")
			policy := metav1.DeletePropagationForeground
			Expect(k8sClient.Delete(context.Background(), testNS, &client.DeleteOptions{PropagationPolicy: &policy})).To(Succeed())
			Eventually(func() error {
				err := k8sClient.Get(context.Background(), client.ObjectKey{Name: testNS.Name}, &corev1.Namespace{})
				if err != nil && apierrors.IsNotFound(err) {
					return nil // Successfully deleted
				}
				return err
			}, 20*time.Second, 500*time.Millisecond).Should(Succeed())

			// 2) Stop manager gracefully: cancel context then wait for goroutine completion
			By("Stopping manager gracefully")
			cancel()
			Eventually(func() bool {
				select {
				case <-doneCh:
					return true
				default:
					return false
				}
			}, 5*time.Second, 50*time.Millisecond).Should(BeTrue())
		})
		It("should successfully reconcile and create ResourceBindings", func() {
			By("Waiting for controller to process the workload creation via Watch")
			Eventually(func(g Gomega) {
				var updatedWorkload scorev1b1.Workload
				g.Expect(k8sCl.Get(context.Background(), typeNamespacedName, &updatedWorkload)).To(Succeed())

				By("Checking that finalizer was added")
				g.Expect(internalreconcile.HasFinalizer(&updatedWorkload)).To(BeTrue())

				By("Checking that InputsValid condition was set")
				inputsCondition := conditions.GetCondition(updatedWorkload.Status.Conditions, conditions.ConditionInputsValid)
				g.Expect(inputsCondition).NotTo(BeNil())
				g.Expect(inputsCondition.Status).To(Equal(metav1.ConditionTrue))
			}).Should(Succeed())

			By("Checking that ResourceBinding was created via Watch")
			Eventually(func(g Gomega) {
				bindingList := &scorev1b1.ResourceBindingList{}
				g.Expect(k8sCl.List(context.Background(), bindingList, client.InNamespace(testNS.Name))).To(Succeed())
				g.Expect(bindingList.Items).ToNot(BeEmpty())

				binding := bindingList.Items[0]
				g.Expect(binding.Spec.Key).To(Equal("db"))
				g.Expect(binding.Spec.Type).To(Equal("postgresql"))
				g.Expect(binding.Spec.WorkloadRef.Name).To(Equal(workload.Name))
				g.Expect(binding.Spec.WorkloadRef.Namespace).To(Equal(workload.Namespace))
			}).Should(Succeed())

			By("Checking that BindingsReady condition is False initially")
			Eventually(func(g Gomega) {
				var updatedWorkload scorev1b1.Workload
				g.Expect(k8sCl.Get(context.Background(), typeNamespacedName, &updatedWorkload)).To(Succeed())
				bindingsCondition := conditions.GetCondition(updatedWorkload.Status.Conditions, conditions.ConditionBindingsReady)
				g.Expect(bindingsCondition).NotTo(BeNil())
				g.Expect(bindingsCondition.Status).To(Equal(metav1.ConditionFalse))
			}).Should(Succeed())
		})

		It("should create WorkloadPlan when bindings are ready", func() {
			// Wait for ResourceBinding to be created (event-driven)
			By("Waiting for ResourceBinding to be created via Watch")
			var bindingKey types.NamespacedName
			Eventually(func(g Gomega) {
				bindingList := &scorev1b1.ResourceBindingList{}
				g.Expect(k8sCl.List(context.Background(), bindingList, client.InNamespace(testNS.Name))).To(Succeed())
				g.Expect(bindingList.Items).ToNot(BeEmpty())
				bindingKey = types.NamespacedName{
					Name:      bindingList.Items[0].Name,
					Namespace: bindingList.Items[0].Namespace,
				}
			}).Should(Succeed())

			By("Updating ResourceBinding status to Bound")
			binding := &scorev1b1.ResourceBinding{}
			Expect(k8sClient.Get(context.Background(), bindingKey, binding)).To(Succeed())

			binding.Status.Phase = scorev1b1.ResourceBindingPhaseBound
			binding.Status.OutputsAvailable = true
			binding.Status.Reason = conditions.ReasonSucceeded
			binding.Status.Message = "Resource provisioned successfully"
			binding.Status.Outputs = scorev1b1.ResourceBindingOutputs{
				URI: stringPtr("postgresql://user:pass@localhost:5432/db"),
			}
			Expect(k8sClient.Status().Update(context.Background(), binding)).To(Succeed())

			By("Waiting for binding status change to propagate to cached client")
			Eventually(func(g Gomega) {
				var updatedBinding scorev1b1.ResourceBinding
				g.Expect(k8sCl.Get(context.Background(), bindingKey, &updatedBinding)).To(Succeed())
				g.Expect(updatedBinding.Status.Phase).To(Equal(scorev1b1.ResourceBindingPhaseBound))
				g.Expect(updatedBinding.Status.OutputsAvailable).To(BeTrue())
			}).Should(Succeed())

			By("Waiting for BindingsReady condition to become True via Watch")
			Eventually(func(g Gomega) {
				var updatedWorkload scorev1b1.Workload
				g.Expect(k8sCl.Get(context.Background(), typeNamespacedName, &updatedWorkload)).To(Succeed())
				bindingsCondition := conditions.GetCondition(updatedWorkload.Status.Conditions, conditions.ConditionBindingsReady)
				g.Expect(bindingsCondition).NotTo(BeNil())
				g.Expect(bindingsCondition.Status).To(Equal(metav1.ConditionTrue))
			}).Should(Succeed())

			By("Waiting for WorkloadPlan to be created via Watch")
			planKey := types.NamespacedName{Name: workload.Name, Namespace: workload.Namespace}
			Eventually(func() error {
				var plan scorev1b1.WorkloadPlan
				return k8sCl.Get(context.Background(), planKey, &plan)
			}).Should(Succeed())

			By("Verifying WorkloadPlan content")
			var finalPlan scorev1b1.WorkloadPlan
			Expect(k8sCl.Get(context.Background(), planKey, &finalPlan)).To(Succeed())
			Expect(finalPlan.Spec.WorkloadRef.Name).To(Equal(workload.Name))
			Expect(finalPlan.Spec.RuntimeClass).To(Equal(meta.RuntimeClassKubernetes))
		})
	})
})
