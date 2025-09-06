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

package envtest

import (
	"context"
	"encoding/json"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/controller"
	"github.com/cappyzawa/score-orchestrator/internal/resolver"
)

// Helper function to create JSON raw message from map
func createJSONRaw(data map[string]interface{}) *apiextv1.JSON {
	jsonBytes, _ := json.Marshal(data)
	return &apiextv1.JSON{Raw: jsonBytes}
}

var _ = Describe("StaticURI Resolver Integration", func() {
	var (
		testCtx       context.Context
		testCancel    context.CancelFunc
		testNamespace string
		testMgr       manager.Manager
		mgrCtx        context.Context
		mgrCancel     context.CancelFunc
	)

	BeforeEach(func() {
		testCtx, testCancel = context.WithCancel(ctx)

		// Create a unique test namespace
		testNamespace = "resolver-test-" + randStringRunes(8)
		testNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Create(testCtx, testNS)).To(Succeed())

		// Setup manager with namespace-scoped cache
		var err error
		testMgr, err = ctrl.NewManager(cfg, ctrl.Options{
			Scheme: k8sClient.Scheme(),
			Cache: cache.Options{
				ByObject: map[client.Object]cache.ByObject{
					&scorev1b1.Workload{}:        {Namespaces: map[string]cache.Config{testNamespace: {}}},
					&scorev1b1.ResourceBinding{}: {Namespaces: map[string]cache.Config{testNamespace: {}}},
					&scorev1b1.WorkloadPlan{}:    {Namespaces: map[string]cache.Config{testNamespace: {}}},
				},
			},
			Metrics: server.Options{
				BindAddress: "0", // Disable metrics
			},
			Controller: config.Controller{
				SkipNameValidation: &[]bool{true}[0], // Allow duplicate controller names in tests
			},
		})
		Expect(err).NotTo(HaveOccurred())

		// Setup indexers for the Orchestrator
		Expect(controller.SetupIndexers(testCtx, testMgr)).To(Succeed())

		// Setup Orchestrator (WorkloadReconciler)
		orchestrator := &controller.WorkloadReconciler{
			Client:   testMgr.GetClient(),
			Scheme:   testMgr.GetScheme(),
			Recorder: testMgr.GetEventRecorderFor("test-orchestrator"),
		}
		Expect(orchestrator.SetupWithManager(testMgr)).To(Succeed())

		// Setup StaticURI Resolver
		staticURIResolver := &resolver.StaticURIController{
			Client:   testMgr.GetClient(),
			Scheme:   testMgr.GetScheme(),
			Recorder: testMgr.GetEventRecorderFor("test-static-uri-resolver"),
		}
		Expect(staticURIResolver.SetupWithManager(testMgr)).To(Succeed())

		// Start the manager
		mgrCtx, mgrCancel = context.WithCancel(testCtx)
		go func() {
			defer GinkgoRecover()
			err := testMgr.Start(mgrCtx)
			Expect(err).ToNot(HaveOccurred(), "failed to run manager")
		}()

		// Wait for the cache to sync
		Expect(testMgr.GetCache().WaitForCacheSync(testCtx)).To(BeTrue())
	})

	AfterEach(func() {
		By("Tearing down manager")
		if mgrCancel != nil {
			mgrCancel()
		}

		By("Cleaning up test resources")
		// Delete all resources in the namespace to avoid finalizer issues
		workloadList := &scorev1b1.WorkloadList{}
		Expect(k8sClient.List(testCtx, workloadList, client.InNamespace(testNamespace))).To(Succeed())
		for i := range workloadList.Items {
			workload := &workloadList.Items[i]
			// Remove finalizers to allow deletion
			workload.Finalizers = nil
			Expect(k8sClient.Update(testCtx, workload)).To(Succeed())
			Expect(k8sClient.Delete(testCtx, workload, client.GracePeriodSeconds(0))).To(Succeed())
		}

		rbList := &scorev1b1.ResourceBindingList{}
		Expect(k8sClient.List(testCtx, rbList, client.InNamespace(testNamespace))).To(Succeed())
		for i := range rbList.Items {
			rb := &rbList.Items[i]
			Expect(k8sClient.Delete(testCtx, rb, client.GracePeriodSeconds(0))).To(Succeed())
		}

		planList := &scorev1b1.WorkloadPlanList{}
		Expect(k8sClient.List(testCtx, planList, client.InNamespace(testNamespace))).To(Succeed())
		for i := range planList.Items {
			plan := &planList.Items[i]
			Expect(k8sClient.Delete(testCtx, plan, client.GracePeriodSeconds(0))).To(Succeed())
		}

		By("Deleting test namespace")
		testNS := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		Expect(k8sClient.Delete(testCtx, testNS, client.GracePeriodSeconds(0))).To(Succeed())

		testCancel()
	})

	It("should complete E2E flow from Workload to WorkloadPlan via static-uri resolver", func() {
		By("Creating a Workload with static-uri resource")
		workloadName := "test-workload"
		testURI := "https://example.com/api/v1"

		workload := &scorev1b1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workloadName,
				Namespace: testNamespace,
			},
			Spec: scorev1b1.WorkloadSpec{
				Containers: map[string]scorev1b1.ContainerSpec{
					"app": {
						Image: "nginx:latest",
					},
				},
				Resources: map[string]scorev1b1.ResourceSpec{
					"api-endpoint": {
						Type: "static-uri",
						Params: createJSONRaw(map[string]interface{}{
							"uri": testURI,
						}),
					},
				},
			},
		}

		Expect(testMgr.GetClient().Create(testCtx, workload)).To(Succeed())

		By("Waiting for Orchestrator to create ResourceBinding")
		rbKey := types.NamespacedName{
			Namespace: testNamespace,
			Name:      workloadName + "-api-endpoint",
		}
		rb := &scorev1b1.ResourceBinding{}
		Eventually(func(g Gomega) {
			err := testMgr.GetClient().Get(testCtx, rbKey, rb)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(rb.Spec.Type).To(Equal("static-uri"))
			g.Expect(rb.Spec.Key).To(Equal("api-endpoint"))
		}, timeout, interval).Should(Succeed())

		By("Waiting for StaticURI Resolver to update ResourceBinding status")
		Eventually(func(g Gomega) {
			err := testMgr.GetClient().Get(testCtx, rbKey, rb)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(rb.Status.Phase).To(Equal(scorev1b1.ResourceBindingPhaseBound))
			g.Expect(rb.Status.OutputsAvailable).To(BeTrue())
			g.Expect(rb.Status.Outputs.URI).ToNot(BeNil())
			g.Expect(*rb.Status.Outputs.URI).To(Equal(testURI))
			g.Expect(rb.Status.Reason).To(Equal("Succeeded"))
		}, timeout, interval).Should(Succeed())

		By("Waiting for Orchestrator to update Workload status to BindingsReady")
		workloadKey := types.NamespacedName{
			Namespace: testNamespace,
			Name:      workloadName,
		}
		Eventually(func(g Gomega) {
			err := testMgr.GetClient().Get(testCtx, workloadKey, workload)
			g.Expect(err).ToNot(HaveOccurred())

			// Check for BindingsReady condition
			bindingsReadyCondition := findCondition(workload.Status.Conditions, "BindingsReady")
			g.Expect(bindingsReadyCondition).ToNot(BeNil())
			g.Expect(bindingsReadyCondition.Status).To(Equal(metav1.ConditionTrue))
		}, timeout, interval).Should(Succeed())

		By("Waiting for Orchestrator to create WorkloadPlan")
		planKey := types.NamespacedName{
			Namespace: testNamespace,
			Name:      workloadName,
		}
		plan := &scorev1b1.WorkloadPlan{}
		Eventually(func(g Gomega) {
			err := testMgr.GetClient().Get(testCtx, planKey, plan)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(plan.Spec.WorkloadRef.Name).To(Equal(workloadName))
			g.Expect(plan.Spec.WorkloadRef.Namespace).To(Equal(testNamespace))
		}, timeout, interval).Should(Succeed())

		By("Verifying WorkloadPlan contains the expected binding information")
		Expect(plan.Spec.Bindings).To(HaveLen(1))
		binding := plan.Spec.Bindings[0]
		Expect(binding.Key).To(Equal("api-endpoint"))
		Expect(binding.Type).To(Equal("static-uri"))
	})

	It("should handle SpecInvalid case when URI param is missing", func() {
		By("Creating a Workload with static-uri resource without URI param")
		workloadName := "test-workload-invalid"

		workload := &scorev1b1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      workloadName,
				Namespace: testNamespace,
			},
			Spec: scorev1b1.WorkloadSpec{
				Containers: map[string]scorev1b1.ContainerSpec{
					"app": {
						Image: "nginx:latest",
					},
				},
				Resources: map[string]scorev1b1.ResourceSpec{
					"api-endpoint": {
						Type: "static-uri",
						// No params provided - this should cause SpecInvalid
					},
				},
			},
		}

		Expect(testMgr.GetClient().Create(testCtx, workload)).To(Succeed())

		By("Waiting for Orchestrator to create ResourceBinding")
		rbKey := types.NamespacedName{
			Namespace: testNamespace,
			Name:      workloadName + "-api-endpoint",
		}
		rb := &scorev1b1.ResourceBinding{}
		Eventually(func(g Gomega) {
			err := testMgr.GetClient().Get(testCtx, rbKey, rb)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(rb.Spec.Type).To(Equal("static-uri"))
			g.Expect(rb.Spec.Key).To(Equal("api-endpoint"))
		}, timeout, interval).Should(Succeed())

		By("Waiting for StaticURI Resolver to update ResourceBinding to Failed state")
		Eventually(func(g Gomega) {
			err := testMgr.GetClient().Get(testCtx, rbKey, rb)
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(rb.Status.Phase).To(Equal(scorev1b1.ResourceBindingPhaseFailed))
			g.Expect(rb.Status.Reason).To(Equal("SpecInvalid"))
			g.Expect(rb.Status.OutputsAvailable).To(BeFalse())
			g.Expect(rb.Status.Message).To(Equal("Required param 'uri' is missing."))
		}, timeout, interval).Should(Succeed())

		By("Verifying that BindingsReady does not become True")
		workloadKey := types.NamespacedName{
			Namespace: testNamespace,
			Name:      workloadName,
		}
		Consistently(func(g Gomega) {
			err := testMgr.GetClient().Get(testCtx, workloadKey, workload)
			g.Expect(err).ToNot(HaveOccurred())

			// Check that BindingsReady condition is either absent or False
			bindingsReadyCondition := findCondition(workload.Status.Conditions, "BindingsReady")
			if bindingsReadyCondition != nil {
				g.Expect(bindingsReadyCondition.Status).ToNot(Equal(metav1.ConditionTrue))
			}
		}, 3*time.Second, 500*time.Millisecond).Should(Succeed())

		By("Verifying that WorkloadPlan is not created")
		planKey := types.NamespacedName{
			Namespace: testNamespace,
			Name:      workloadName,
		}
		plan := &scorev1b1.WorkloadPlan{}
		Consistently(func() bool {
			err := testMgr.GetClient().Get(testCtx, planKey, plan)
			return errors.IsNotFound(err)
		}, 3*time.Second, 500*time.Millisecond).Should(BeTrue())
	})
})

// Helper function to find condition by type
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for i := range conditions {
		if conditions[i].Type == conditionType {
			return &conditions[i]
		}
	}
	return nil
}
