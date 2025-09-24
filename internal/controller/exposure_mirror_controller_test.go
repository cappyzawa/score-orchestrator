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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

var _ = Describe("ExposureMirrorReconciler", func() {
	var (
		reconciler *ExposureMirrorReconciler
		ctx        context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		reconciler = &ExposureMirrorReconciler{
			Client:   k8sClient,
			Scheme:   k8sClient.Scheme(),
			Recorder: record.NewFakeRecorder(10),
		}
	})

	Context("when reconciling WorkloadExposure", func() {
		var (
			workload *scorev1b1.Workload
			exposure *scorev1b1.WorkloadExposure
			req      ctrl.Request
		)

		BeforeEach(func() {
			workload = &scorev1b1.Workload{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test-workload",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Image: "nginx:latest",
						},
					},
				},
				Status: scorev1b1.WorkloadStatus{
					Endpoint: nil,
				},
			}

			exposure = &scorev1b1.WorkloadExposure{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-exposure",
					Namespace: "default",
				},
				Spec: scorev1b1.WorkloadExposureSpec{
					WorkloadRef: scorev1b1.WorkloadExposureWorkloadRef{
						Name:      "test-workload",
						Namespace: stringPtr("default"),
					},
					ObservedWorkloadGeneration: 1,
				},
			}

			req = ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      exposure.Name,
					Namespace: exposure.Namespace,
				},
			}

			Expect(k8sClient.Create(ctx, workload)).To(Succeed())
			Expect(k8sClient.Create(ctx, exposure)).To(Succeed())
		})

		AfterEach(func() {
			Expect(k8sClient.Delete(ctx, workload)).To(Succeed())
			Expect(k8sClient.Delete(ctx, exposure)).To(Succeed())
		})

		When("exposure has a valid ready URL", func() {
			BeforeEach(func() {
				exposure.Status = scorev1b1.WorkloadExposureStatus{
					Exposures: []scorev1b1.ExposureEntry{
						{
							URL:   "https://example.com",
							Ready: true,
						},
					},
				}
				Expect(k8sClient.Status().Update(ctx, exposure)).To(Succeed())
			})

			It("should mirror the URL to workload endpoint", func() {
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var updatedWorkload scorev1b1.Workload
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      workload.Name,
					Namespace: workload.Namespace,
				}, &updatedWorkload)).To(Succeed())

				Expect(updatedWorkload.Status.Endpoint).NotTo(BeNil())
				Expect(*updatedWorkload.Status.Endpoint).To(Equal("https://example.com"))
			})
		})

		When("exposure has invalid URL in first position", func() {
			BeforeEach(func() {
				exposure.Status = scorev1b1.WorkloadExposureStatus{
					Exposures: []scorev1b1.ExposureEntry{
						{
							URL:   "https://", // Invalid - no host
							Ready: true,
						},
					},
				}
				Expect(k8sClient.Status().Update(ctx, exposure)).To(Succeed())
			})

			It("should ignore the invalid URL", func() {
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var updatedWorkload scorev1b1.Workload
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      workload.Name,
					Namespace: workload.Namespace,
				}, &updatedWorkload)).To(Succeed())

				Expect(updatedWorkload.Status.Endpoint).To(BeNil())
			})
		})

		When("exposure has outdated generation", func() {
			BeforeEach(func() {
				// Update workload generation
				workload.Generation = 2
				workload.Spec = scorev1b1.WorkloadSpec{
					Containers: map[string]scorev1b1.ContainerSpec{
						"app": {
							Image: "nginx:latest",
						},
					},
				}
				Expect(k8sClient.Update(ctx, workload)).To(Succeed())

				exposure.Spec.ObservedWorkloadGeneration = 1 // Older generation
				exposure.Status = scorev1b1.WorkloadExposureStatus{
					Exposures: []scorev1b1.ExposureEntry{
						{
							URL:   "https://example.com",
							Ready: true,
						},
					},
				}
				Expect(k8sClient.Update(ctx, exposure)).To(Succeed())
				Expect(k8sClient.Status().Update(ctx, exposure)).To(Succeed())
			})

			It("should skip processing", func() {
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var updatedWorkload scorev1b1.Workload
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      workload.Name,
					Namespace: workload.Namespace,
				}, &updatedWorkload)).To(Succeed())

				Expect(updatedWorkload.Status.Endpoint).To(BeNil())
			})
		})

		When("workload already has the same endpoint", func() {
			BeforeEach(func() {
				workload.Status.Endpoint = stringPtr("https://example.com")
				Expect(k8sClient.Status().Update(ctx, workload)).To(Succeed())

				exposure.Status = scorev1b1.WorkloadExposureStatus{
					Exposures: []scorev1b1.ExposureEntry{
						{
							URL:   "https://example.com",
							Ready: true,
						},
					},
				}
				Expect(k8sClient.Status().Update(ctx, exposure)).To(Succeed())
			})

			It("should not update the workload unnecessarily", func() {
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var updatedWorkload scorev1b1.Workload
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      workload.Name,
					Namespace: workload.Namespace,
				}, &updatedWorkload)).To(Succeed())

				Expect(updatedWorkload.Status.Endpoint).NotTo(BeNil())
				Expect(*updatedWorkload.Status.Endpoint).To(Equal("https://example.com"))
			})
		})

		When("exposure has multiple URLs", func() {
			BeforeEach(func() {
				exposure.Status = scorev1b1.WorkloadExposureStatus{
					Exposures: []scorev1b1.ExposureEntry{
						{
							URL:   "https://primary.example.com",
							Ready: false, // Runtime ordered this first regardless of ready state
						},
						{
							URL:   "https://secondary.example.com",
							Ready: true,
						},
					},
				}
				Expect(k8sClient.Status().Update(ctx, exposure)).To(Succeed())
			})

			It("should use exposures[0] regardless of ready state (mirror-only)", func() {
				result, err := reconciler.Reconcile(ctx, req)
				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal(ctrl.Result{}))

				var updatedWorkload scorev1b1.Workload
				Expect(k8sClient.Get(ctx, types.NamespacedName{
					Name:      workload.Name,
					Namespace: workload.Namespace,
				}, &updatedWorkload)).To(Succeed())

				Expect(updatedWorkload.Status.Endpoint).NotTo(BeNil())
				Expect(*updatedWorkload.Status.Endpoint).To(Equal("https://primary.example.com"))
			})
		})
	})

	Context("when validating URLs", func() {
		DescribeTable("URL validation",
			func(url string, expected bool) {
				Expect(isValidURL(url)).To(Equal(expected))
			},
			Entry("valid HTTPS URL", "https://example.com", true),
			Entry("valid HTTP URL", "http://example.com", true),
			Entry("valid URL with path", "https://example.com/path", true),
			Entry("valid URL with port", "http://example.com:8080", true),
			Entry("empty URL", "", false),
			Entry("URL without host (https)", "https://", false),
			Entry("URL without host (http)", "http://", false),
			Entry("invalid scheme", "ftp://example.com", false),
		)

		It("should validate URLs correctly in isolation", func() {
			// Test some edge cases not covered in the table
			Expect(isValidURL("http://localhost")).To(BeTrue())                         // localhost
			Expect(isValidURL("https://example.com/")).To(BeTrue())                     // trailing slash
			Expect(isValidURL("https://example.com:443/path?query=value")).To(BeTrue()) // complex URL

			// Test invalid URLs without host
			Expect(isValidURL("https://")).To(BeFalse())          // no host
			Expect(isValidURL("http://")).To(BeFalse())           // no host
			Expect(isValidURL("ftp://example.com")).To(BeFalse()) // invalid scheme
		})
	})

	Context("when mirroring conditions", func() {
		var (
			workload *scorev1b1.Workload
			exposure *scorev1b1.WorkloadExposure
		)

		BeforeEach(func() {
			workload = &scorev1b1.Workload{
				Status: scorev1b1.WorkloadStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "InputsValid",
							Status: metav1.ConditionTrue,
							Reason: "Succeeded",
						},
					},
				},
			}

			exposure = &scorev1b1.WorkloadExposure{
				Status: scorev1b1.WorkloadExposureStatus{
					Conditions: []metav1.Condition{
						{
							Type:   "Ready",
							Status: metav1.ConditionTrue,
							Reason: "Available", // Will be normalized to "Succeeded"
						},
					},
				},
			}
		})

		It("should normalize and merge conditions", func() {
			updated := reconciler.mirrorConditions(workload, exposure)
			Expect(updated).To(BeTrue())

			Expect(workload.Status.Conditions).To(HaveLen(2))

			var readyCondition *metav1.Condition
			for i := range workload.Status.Conditions {
				if workload.Status.Conditions[i].Type == "Ready" {
					readyCondition = &workload.Status.Conditions[i]
					break
				}
			}

			Expect(readyCondition).NotTo(BeNil())
			Expect(readyCondition.Reason).To(Equal("Succeeded")) // Should be normalized
		})
	})
})
