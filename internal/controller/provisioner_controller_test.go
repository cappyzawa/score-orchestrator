package controller

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/config"
)

var (
	provisionerScheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(provisionerScheme))
	utilruntime.Must(scorev1b1.AddToScheme(provisionerScheme))
}

var _ = Describe("ProvisionerController", func() {
	var (
		ctx              context.Context
		cancel           context.CancelFunc
		reconciler       *ProvisionerReconciler
		mockConfigLoader *config.MockLoader
		mockStrategy     *MockStrategy
	)

	BeforeEach(func() {
		ctx, cancel = context.WithCancel(context.Background())

		// Set supported types for testing
		Expect(os.Setenv("SUPPORTED_RESOURCE_TYPES", "test,mock")).To(Succeed())

		// Create mock config loader
		mockConfigLoader = &config.MockLoader{}

		// Create reconciler using the shared k8sClient from suite_test.go
		reconciler = NewProvisionerReconciler(
			k8sClient,
			provisionerScheme,
			record.NewFakeRecorder(100),
			mockConfigLoader,
		)

		// Create and register mock strategy
		mockStrategy = &MockStrategy{}
		reconciler.StrategySelector.RegisterStrategy(mockStrategy)

		// Set supported types for reconciler
		reconciler.supportedTypes = map[string]bool{"test": true, "mock": true}
	})

	AfterEach(func() {
		cancel()
		Expect(os.Unsetenv("SUPPORTED_RESOURCE_TYPES")).To(Succeed())
	})

	Context("When reconciling a ResourceClaim", func() {
		var (
			resourceClaim *scorev1b1.ResourceClaim
			namespaceName types.NamespacedName
		)

		createResourceClaim := func(name string) {
			resourceClaim = &scorev1b1.ResourceClaim{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: "default",
				},
				Spec: scorev1b1.ResourceClaimSpec{
					WorkloadRef: scorev1b1.NamespacedName{
						Name:      "test-workload",
						Namespace: "default",
					},
					Key:  "test-resource",
					Type: "test",
				},
			}
			namespaceName = types.NamespacedName{
				Name:      resourceClaim.Name,
				Namespace: resourceClaim.Namespace,
			}
		}

		It("Should add finalizer to new ResourceClaim", func() {
			createResourceClaim("test-claim-finalizer")
			By("Creating a new ResourceClaim")
			Expect(k8sClient.Create(ctx, resourceClaim)).To(Succeed())

			By("Reconciling the ResourceClaim")
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespaceName})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that finalizer was added")
			updatedClaim := &scorev1b1.ResourceClaim{}
			Expect(k8sClient.Get(ctx, namespaceName, updatedClaim)).To(Succeed())
			Expect(updatedClaim.Finalizers).To(ContainElement(ResourceClaimFinalizer))
		})

		It("Should transition from Pending to Bound with successful provisioning", func() {
			createResourceClaim("test-claim-pending")
			By("Creating a ResourceClaim with finalizer")
			resourceClaim.Finalizers = []string{ResourceClaimFinalizer}
			Expect(k8sClient.Create(ctx, resourceClaim)).To(Succeed())

			By("Configuring mock strategy to return successful outputs")
			mockStrategy.SetStatus(scorev1b1.ResourceClaimPhaseBound, ReasonSucceeded, "Resource provisioned")
			mockStrategy.SetOutputs(&scorev1b1.ResourceClaimOutputs{
				URI: StringPtr("test://localhost:1234"),
			})

			By("Reconciling the ResourceClaim")
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespaceName})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that phase transitioned to Bound")
			updatedClaim := &scorev1b1.ResourceClaim{}
			Expect(k8sClient.Get(ctx, namespaceName, updatedClaim)).To(Succeed())
			Expect(updatedClaim.Status.Phase).To(Equal(scorev1b1.ResourceClaimPhaseBound))
			Expect(updatedClaim.Status.OutputsAvailable).To(BeTrue())
		})

		It("Should transition to Bound with valid outputs", func() {
			createResourceClaim("test-claim-bound")
			By("Creating a ResourceClaim in Claiming phase")
			resourceClaim.Finalizers = []string{ResourceClaimFinalizer}
			Expect(k8sClient.Create(ctx, resourceClaim)).To(Succeed())

			By("Updating the status to Claiming phase")
			resourceClaim.Status.Phase = scorev1b1.ResourceClaimPhaseClaiming
			Expect(k8sClient.Status().Update(ctx, resourceClaim)).To(Succeed())

			By("Configuring mock strategy to return Bound status with outputs")
			outputs := &scorev1b1.ResourceClaimOutputs{
				URI: StringPtr("test://localhost:1234"),
			}
			mockStrategy.SetStatus(scorev1b1.ResourceClaimPhaseBound, ReasonSucceeded, "Resource provisioned")
			mockStrategy.SetOutputs(outputs)

			By("Reconciling the ResourceClaim")
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespaceName})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that phase transitioned to Bound with outputs")
			updatedClaim := &scorev1b1.ResourceClaim{}
			Expect(k8sClient.Get(ctx, namespaceName, updatedClaim)).To(Succeed())
			Expect(updatedClaim.Status.Phase).To(Equal(scorev1b1.ResourceClaimPhaseBound))
			Expect(updatedClaim.Status.OutputsAvailable).To(BeTrue())
			Expect(updatedClaim.Status.Outputs.URI).To(Equal(StringPtr("test://localhost:1234")))
		})

		It("Should handle deletion with finalizer cleanup", func() {
			createResourceClaim("test-claim-deletion")
			By("Creating a ResourceClaim with finalizer")
			resourceClaim.Finalizers = []string{ResourceClaimFinalizer}
			Expect(k8sClient.Create(ctx, resourceClaim)).To(Succeed())

			By("Deleting the ResourceClaim")
			Expect(k8sClient.Delete(ctx, resourceClaim)).To(Succeed())

			By("Reconciling the ResourceClaim")
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespaceName})
			Expect(err).NotTo(HaveOccurred())

			By("Checking that finalizer was removed and ResourceClaim is deleted")
			updatedClaim := &scorev1b1.ResourceClaim{}
			err = k8sClient.Get(ctx, namespaceName, updatedClaim)
			Expect(err).To(HaveOccurred())
			Expect(client.IgnoreNotFound(err)).To(Succeed())
		})

		It("Should handle failed provisioning", func() {
			createResourceClaim("test-claim-failed")
			By("Creating a ResourceClaim in Claiming phase")
			resourceClaim.Finalizers = []string{ResourceClaimFinalizer}
			Expect(k8sClient.Create(ctx, resourceClaim)).To(Succeed())

			By("Updating the status to Claiming phase")
			resourceClaim.Status.Phase = scorev1b1.ResourceClaimPhaseClaiming
			Expect(k8sClient.Status().Update(ctx, resourceClaim)).To(Succeed())

			By("Configuring mock strategy to return Failed status")
			mockStrategy.SetStatus(scorev1b1.ResourceClaimPhaseFailed, ReasonClaimFailed, "Provisioning failed")

			By("Reconciling the ResourceClaim")
			_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: namespaceName})
			Expect(err).To(HaveOccurred())

			By("Checking that phase transitioned to Failed")
			updatedClaim := &scorev1b1.ResourceClaim{}
			Expect(k8sClient.Get(ctx, namespaceName, updatedClaim)).To(Succeed())
			Expect(updatedClaim.Status.Phase).To(Equal(scorev1b1.ResourceClaimPhaseFailed))
			Expect(updatedClaim.Status.OutputsAvailable).To(BeFalse())
		})
	})

	Context("When filtering ResourceClaims", func() {
		It("Should only reconcile supported resource types", func() {
			unsupportedClaim := &scorev1b1.ResourceClaim{
				Spec: scorev1b1.ResourceClaimSpec{
					Type: "unsupported",
				},
			}

			supportedClaim := &scorev1b1.ResourceClaim{
				Spec: scorev1b1.ResourceClaimSpec{
					Type: "test",
				},
			}

			Expect(reconciler.filterSupportedTypes(unsupportedClaim)).To(BeFalse())
			Expect(reconciler.filterSupportedTypes(supportedClaim)).To(BeTrue())
		})
	})
})

// MockStrategy implements the strategy.Strategy interface for testing
type MockStrategy struct {
	phase   scorev1b1.ResourceClaimPhase
	reason  string
	message string
	outputs *scorev1b1.ResourceClaimOutputs
	err     error
}

func (m *MockStrategy) GetType() string {
	return "test"
}

func (m *MockStrategy) Provision(ctx context.Context, claim *scorev1b1.ResourceClaim) (*scorev1b1.ResourceClaimOutputs, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.outputs, nil
}

func (m *MockStrategy) Deprovision(ctx context.Context, claim *scorev1b1.ResourceClaim) error {
	return m.err
}

func (m *MockStrategy) GetStatus(ctx context.Context, claim *scorev1b1.ResourceClaim) (scorev1b1.ResourceClaimPhase, string, string, error) {
	if m.err != nil {
		return "", "", "", m.err
	}
	return m.phase, m.reason, m.message, nil
}

func (m *MockStrategy) SetStatus(phase scorev1b1.ResourceClaimPhase, reason, message string) {
	m.phase = phase
	m.reason = reason
	m.message = message
}

func (m *MockStrategy) SetOutputs(outputs *scorev1b1.ResourceClaimOutputs) {
	m.outputs = outputs
}

func (m *MockStrategy) SetError(err error) {
	m.err = err
}

// StringPtr returns a pointer to a string
func StringPtr(s string) *string {
	return &s
}
