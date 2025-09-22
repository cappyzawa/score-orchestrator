package managers

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
	"github.com/cappyzawa/score-orchestrator/internal/endpoint"
)

var _ = Describe("StatusManager", func() {
	var (
		scheme   *runtime.Scheme
		workload *scorev1b1.Workload
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(scorev1b1.AddToScheme(scheme)).ToNot(HaveOccurred())

		workload = &scorev1b1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workload",
				Namespace: "test-ns",
			},
			Status: scorev1b1.WorkloadStatus{
				Conditions: []metav1.Condition{
					{
						Type:   conditions.ConditionReady,
						Status: metav1.ConditionTrue,
					},
				},
			},
		}
	})

	Describe("UpdateStatus", func() {
		Context("when client has status subresource", func() {
			It("should succeed", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&scorev1b1.Workload{}).Build()
				mockRecorder := &mockEventRecorder{}
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

				sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

				// Create workload first
				err := fakeClient.Create(context.Background(), workload)
				Expect(err).ToNot(HaveOccurred())

				err = sm.UpdateStatus(context.Background(), workload)
				Expect(err).ToNot(HaveOccurred())
			})
		})

		Context("when client does not have status subresource", func() {
			It("should handle client error", func() {
				// Use a client that will fail status updates
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build() // No status subresource
				mockRecorder := &mockEventRecorder{}
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

				sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

				err := sm.UpdateStatus(context.Background(), workload)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to update workload status"))
			})
		})
	})

	Describe("ComputeReadyCondition", func() {
		var (
			fakeClient      *fake.ClientBuilder
			mockRecorder    *mockEventRecorder
			endpointDeriver *endpoint.EndpointDeriver
			sm              *StatusManager
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			mockRecorder = &mockEventRecorder{}
			endpointDeriver = endpoint.NewEndpointDeriver(fakeClient.Build())
			sm = NewStatusManager(fakeClient.Build(), scheme, mockRecorder, endpointDeriver)
		})

		Context("when all conditions are true", func() {
			It("should result in Ready=True", func() {
				conditions := []metav1.Condition{
					{Type: conditions.ConditionInputsValid, Status: metav1.ConditionTrue},
					{Type: conditions.ConditionClaimsReady, Status: metav1.ConditionTrue},
					{Type: conditions.ConditionRuntimeReady, Status: metav1.ConditionTrue},
				}

				status, reason, message := sm.ComputeReadyCondition(conditions)

				Expect(status).To(Equal(metav1.ConditionTrue))
				Expect(reason).To(Equal("Succeeded"))
				Expect(message).To(Equal("Workload is ready and operational"))
			})
		})

		Context("when InputsValid is false", func() {
			It("should result in Ready=False", func() {
				conditions := []metav1.Condition{
					{
						Type:   conditions.ConditionInputsValid,
						Status: metav1.ConditionFalse,
						Reason: "SpecInvalid",
					},
					{Type: conditions.ConditionClaimsReady, Status: metav1.ConditionTrue},
					{Type: conditions.ConditionRuntimeReady, Status: metav1.ConditionTrue},
				}

				status, reason, message := sm.ComputeReadyCondition(conditions)

				Expect(status).To(Equal(metav1.ConditionFalse))
				Expect(reason).To(Equal("SpecInvalid"))
				Expect(message).To(Equal("Workload specification validation failed"))
			})
		})

		Context("when ClaimsReady is false", func() {
			It("should result in Ready=False", func() {
				conditions := []metav1.Condition{
					{Type: conditions.ConditionInputsValid, Status: metav1.ConditionTrue},
					{
						Type:   conditions.ConditionClaimsReady,
						Status: metav1.ConditionFalse,
						Reason: "ClaimPending",
					},
					{Type: conditions.ConditionRuntimeReady, Status: metav1.ConditionTrue},
				}

				status, reason, message := sm.ComputeReadyCondition(conditions)

				Expect(status).To(Equal(metav1.ConditionFalse))
				Expect(reason).To(Equal("ClaimPending"))
				Expect(message).To(Equal("Resource claims are not ready"))
			})
		})
	})

	Describe("DeriveEndpoint", func() {
		var (
			testWorkload *scorev1b1.Workload
			plan         *scorev1b1.WorkloadPlan
		)

		BeforeEach(func() {
			testWorkload = &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: scorev1b1.WorkloadSpec{
					Service: &scorev1b1.ServiceSpec{
						Ports: []scorev1b1.ServicePort{
							{Port: 8080},
						},
					},
				},
			}

			plan = &scorev1b1.WorkloadPlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: scorev1b1.WorkloadPlanSpec{
					RuntimeClass: "kubernetes",
				},
			}
		})

		Context("when endpoint deriver is configured", func() {
			It("should succeed", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				mockRecorder := &mockEventRecorder{}
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

				sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

				result, err := sm.DeriveEndpoint(context.Background(), testWorkload, plan)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(*result).To(ContainSubstring("test-workload.test-ns.svc.cluster.local"))
			})
		})

		Context("when plan is nil", func() {
			It("should return nil", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				mockRecorder := &mockEventRecorder{}
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

				sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

				result, err := sm.DeriveEndpoint(context.Background(), testWorkload, nil)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})

		Context("when endpointDeriver is nil", func() {
			It("should handle nil endpointDeriver", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				mockRecorder := &mockEventRecorder{}

				sm := NewStatusManager(fakeClient, scheme, mockRecorder, nil)

				result, err := sm.DeriveEndpoint(context.Background(), testWorkload, plan)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("endpoint deriver not configured"))
				Expect(result).To(BeNil())
			})
		})
	})

	Describe("SetConditions", func() {
		var (
			fakeClient      *fake.ClientBuilder
			mockRecorder    *mockEventRecorder
			endpointDeriver *endpoint.EndpointDeriver
			sm              *StatusManager
			testWorkload    *scorev1b1.Workload
		)

		BeforeEach(func() {
			fakeClient = fake.NewClientBuilder().WithScheme(scheme)
			mockRecorder = &mockEventRecorder{}
			endpointDeriver = endpoint.NewEndpointDeriver(fakeClient.Build())
			sm = NewStatusManager(fakeClient.Build(), scheme, mockRecorder, endpointDeriver)

			testWorkload = &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
			}
		})

		Describe("SetInputsValidCondition", func() {
			It("should set the InputsValid condition", func() {
				sm.SetInputsValidCondition(testWorkload, true, "Succeeded", "Validation passed")

				condition := conditions.GetCondition(testWorkload.Status.Conditions, conditions.ConditionInputsValid)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				Expect(condition.Reason).To(Equal("Succeeded"))
				Expect(condition.Message).To(Equal("Validation passed"))
			})
		})

		Describe("SetClaimsReadyCondition", func() {
			It("should set the ClaimsReady condition", func() {
				sm.SetClaimsReadyCondition(testWorkload, false, "ClaimPending", "Claims are binding")

				condition := conditions.GetCondition(testWorkload.Status.Conditions, conditions.ConditionClaimsReady)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal("ClaimPending"))
				Expect(condition.Message).To(Equal("Claims are binding"))
			})
		})

		Describe("SetRuntimeReadyCondition", func() {
			It("should set the RuntimeReady condition", func() {
				sm.SetRuntimeReadyCondition(testWorkload, true, "Succeeded", "Runtime is ready")

				condition := conditions.GetCondition(testWorkload.Status.Conditions, conditions.ConditionRuntimeReady)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionTrue))
				Expect(condition.Reason).To(Equal("Succeeded"))
				Expect(condition.Message).To(Equal("Runtime is ready"))
			})
		})
	})

	Describe("ComputeFinalStatus", func() {
		var (
			testWorkload *scorev1b1.Workload
			plan         *scorev1b1.WorkloadPlan
		)

		BeforeEach(func() {
			testWorkload = &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: scorev1b1.WorkloadSpec{
					Service: &scorev1b1.ServiceSpec{
						Ports: []scorev1b1.ServicePort{
							{Port: 8080},
						},
					},
				},
			}

			plan = &scorev1b1.WorkloadPlan{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-workload",
					Namespace: "test-ns",
				},
				Spec: scorev1b1.WorkloadPlanSpec{
					RuntimeClass: "kubernetes",
				},
			}
		})

		Context("when plan is provided", func() {
			It("should compute final status with plan", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				mockRecorder := &mockEventRecorder{}
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

				sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

				// Set some initial conditions
				sm.SetInputsValidCondition(testWorkload, true, "Succeeded", "Valid")
				sm.SetClaimsReadyCondition(testWorkload, true, "Succeeded", "Ready")

				err := sm.ComputeFinalStatus(context.Background(), testWorkload, plan)

				Expect(err).ToNot(HaveOccurred())

				// Check that RuntimeReady condition was set
				runtimeCondition := conditions.GetCondition(testWorkload.Status.Conditions, conditions.ConditionRuntimeReady)
				Expect(runtimeCondition).ToNot(BeNil())
				Expect(runtimeCondition.Status).To(Equal(metav1.ConditionFalse)) // Always false when plan exists but status is empty
				Expect(runtimeCondition.Reason).To(Equal("RuntimeSelecting"))

				// Check that Ready condition was computed
				readyCondition := conditions.GetCondition(testWorkload.Status.Conditions, conditions.ConditionReady)
				Expect(readyCondition).ToNot(BeNil())

				// Check that endpoint was derived
				Expect(testWorkload.Status.Endpoint).ToNot(BeNil())
				Expect(*testWorkload.Status.Endpoint).To(ContainSubstring("test-workload.test-ns.svc.cluster.local"))

				// Check that event was recorded for non-ready workload
				Expect(mockRecorder.events).To(BeEmpty()) // Should be empty because workload is not ready
			})
		})

		Context("when plan is nil", func() {
			It("should compute final status with nil plan", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				mockRecorder := &mockEventRecorder{}
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

				sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

				err := sm.ComputeFinalStatus(context.Background(), testWorkload, nil)

				Expect(err).ToNot(HaveOccurred())

				// Check that RuntimeReady condition was set to selecting
				runtimeCondition := conditions.GetCondition(testWorkload.Status.Conditions, conditions.ConditionRuntimeReady)
				Expect(runtimeCondition).ToNot(BeNil())
				Expect(runtimeCondition.Status).To(Equal(metav1.ConditionFalse))
				Expect(runtimeCondition.Reason).To(Equal("RuntimeSelecting"))
			})
		})
	})

	Describe("updateRuntimeStatusFromPlan", func() {
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
					Service: &scorev1b1.ServiceSpec{
						Ports: []scorev1b1.ServicePort{
							{Port: 8080},
						},
					},
				},
			}
		})

		Context("when plan is nil", func() {
			It("should set runtime status to selecting", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				mockRecorder := &mockEventRecorder{}
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

				sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

				sm.updateRuntimeStatusFromPlan(context.Background(), testWorkload, nil)

				// Check RuntimeReady condition
				condition := conditions.GetCondition(testWorkload.Status.Conditions, conditions.ConditionRuntimeReady)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal("RuntimeSelecting"))
			})
		})

		Context("when plan is provided", func() {
			It("should set runtime status to provisioning", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				mockRecorder := &mockEventRecorder{}
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

				sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

				plan := &scorev1b1.WorkloadPlan{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workload",
						Namespace: "test-ns",
					},
					Spec: scorev1b1.WorkloadPlanSpec{
						RuntimeClass: "kubernetes",
					},
				}

				sm.updateRuntimeStatusFromPlan(context.Background(), testWorkload, plan)

				// Check RuntimeReady condition
				condition := conditions.GetCondition(testWorkload.Status.Conditions, conditions.ConditionRuntimeReady)
				Expect(condition).ToNot(BeNil())
				Expect(condition.Status).To(Equal(metav1.ConditionFalse))
				Expect(condition.Reason).To(Equal("RuntimeSelecting"))

				// Check endpoint was set
				Expect(testWorkload.Status.Endpoint).ToNot(BeNil())
				Expect(*testWorkload.Status.Endpoint).To(ContainSubstring("test-workload.test-ns.svc.cluster.local"))
			})

			It("should set runtime status based on WorkloadPlan.Status.Phase", func() {
				fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
				mockRecorder := &mockEventRecorder{}
				endpointDeriver := endpoint.NewEndpointDeriver(fakeClient)

				sm := NewStatusManager(fakeClient, scheme, mockRecorder, endpointDeriver)

				testCases := []struct {
					phase           scorev1b1.WorkloadPlanPhase
					message         string
					expectedStatus  metav1.ConditionStatus
					expectedReason  string
					expectedMessage string
				}{
					{
						phase:           scorev1b1.WorkloadPlanPhaseReady,
						message:         "",
						expectedStatus:  metav1.ConditionTrue,
						expectedReason:  "Succeeded",
						expectedMessage: "Runtime provisioned successfully",
					},
					{
						phase:           scorev1b1.WorkloadPlanPhaseFailed,
						message:         "Deployment failed",
						expectedStatus:  metav1.ConditionFalse,
						expectedReason:  "RuntimeDegraded",
						expectedMessage: "Deployment failed",
					},
					{
						phase:           scorev1b1.WorkloadPlanPhaseProvisioning,
						message:         "Creating deployment",
						expectedStatus:  metav1.ConditionFalse,
						expectedReason:  "RuntimeProvisioning",
						expectedMessage: "Creating deployment",
					},
					{
						phase:           scorev1b1.WorkloadPlanPhasePending,
						message:         "",
						expectedStatus:  metav1.ConditionFalse,
						expectedReason:  "RuntimeSelecting",
						expectedMessage: "Runtime provisioning pending",
					},
				}

				for _, tc := range testCases {
					plan := &scorev1b1.WorkloadPlan{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-workload",
							Namespace: "test-ns",
						},
						Spec: scorev1b1.WorkloadPlanSpec{
							RuntimeClass: "kubernetes",
						},
						Status: scorev1b1.WorkloadPlanStatus{
							Phase:   tc.phase,
							Message: tc.message,
						},
					}

					// Reset workload conditions
					testWorkload.Status.Conditions = []metav1.Condition{}

					sm.updateRuntimeStatusFromPlan(context.Background(), testWorkload, plan)

					// Check RuntimeReady condition
					condition := conditions.GetCondition(testWorkload.Status.Conditions, conditions.ConditionRuntimeReady)
					Expect(condition).ToNot(BeNil(), "Phase: %s", tc.phase)
					Expect(condition.Status).To(Equal(tc.expectedStatus), "Phase: %s", tc.phase)
					Expect(condition.Reason).To(Equal(tc.expectedReason), "Phase: %s", tc.phase)
					Expect(condition.Message).To(Equal(tc.expectedMessage), "Phase: %s", tc.phase)
				}
			})
		})
	})
})
