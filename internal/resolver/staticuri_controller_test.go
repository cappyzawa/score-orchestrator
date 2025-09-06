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

package resolver

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// Helper function to create JSON raw message from map
func createJSONRaw(data map[string]interface{}) *apiextv1.JSON {
	jsonBytes, _ := json.Marshal(data)
	return &apiextv1.JSON{Raw: jsonBytes}
}

const testURI = "https://example.com/api"

var _ = Describe("StaticURIController", func() {
	var controller *StaticURIController
	var fakeRecorder *record.FakeRecorder

	BeforeEach(func() {
		fakeRecorder = record.NewFakeRecorder(10)
		controller = &StaticURIController{
			Recorder: fakeRecorder,
		}
	})

	Describe("extractURI", func() {
		Context("with valid params", func() {
			It("should extract URI from params", func() {
				rb := &scorev1b1.ResourceBinding{
					Spec: scorev1b1.ResourceBindingSpec{
						Type: "static-uri",
						Params: createJSONRaw(map[string]interface{}{
							"uri": testURI,
						}),
					},
				}

				uri, missing, err := controller.extractURI(rb)
				Expect(err).ToNot(HaveOccurred())
				Expect(missing).To(BeFalse())
				Expect(uri).To(Equal(testURI))
			})
		})

		Context("with nil params", func() {
			It("should return error", func() {
				rb := &scorev1b1.ResourceBinding{
					Spec: scorev1b1.ResourceBindingSpec{
						Type:   "static-uri",
						Params: nil,
					},
				}

				uri, missing, err := controller.extractURI(rb)
				Expect(err).ToNot(HaveOccurred())
				Expect(missing).To(BeTrue())
				Expect(uri).To(Equal(""))
			})
		})

		Context("with invalid JSON params", func() {
			It("should return error", func() {
				rb := &scorev1b1.ResourceBinding{
					Spec: scorev1b1.ResourceBindingSpec{
						Type: "static-uri",
						Params: &apiextv1.JSON{
							Raw: []byte("{invalid json"),
						},
					},
				}

				_, _, err := controller.extractURI(rb)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("failed to unmarshal params"))
			})
		})

		Context("with missing uri in params", func() {
			It("should return error", func() {
				rb := &scorev1b1.ResourceBinding{
					Spec: scorev1b1.ResourceBindingSpec{
						Type: "static-uri",
						Params: createJSONRaw(map[string]interface{}{
							"other": "value",
						}),
					},
				}

				uri, missing, err := controller.extractURI(rb)
				Expect(err).ToNot(HaveOccurred())
				Expect(missing).To(BeTrue())
				Expect(uri).To(Equal(""))
			})
		})

		Context("with non-string uri in params", func() {
			It("should return error", func() {
				rb := &scorev1b1.ResourceBinding{
					Spec: scorev1b1.ResourceBindingSpec{
						Type: "static-uri",
						Params: createJSONRaw(map[string]interface{}{
							"uri": 12345,
						}),
					},
				}

				uri, missing, err := controller.extractURI(rb)
				Expect(err).ToNot(HaveOccurred())
				Expect(missing).To(BeTrue())
				Expect(uri).To(Equal(""))
			})
		})

		Context("with empty string uri in params", func() {
			It("should return error", func() {
				rb := &scorev1b1.ResourceBinding{
					Spec: scorev1b1.ResourceBindingSpec{
						Type: "static-uri",
						Params: createJSONRaw(map[string]interface{}{
							"uri": "",
						}),
					},
				}

				uri, missing, err := controller.extractURI(rb)
				Expect(err).ToNot(HaveOccurred())
				Expect(missing).To(BeFalse())
				Expect(uri).To(Equal(""))
			})
		})
	})

	Describe("isAlreadyInBoundState", func() {
		Context("with bound ResourceBinding", func() {
			It("should return true when all conditions match", func() {
				expectedURI := testURI
				rb := &scorev1b1.ResourceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 2,
					},
					Status: scorev1b1.ResourceBindingStatus{
						Phase:            scorev1b1.ResourceBindingPhaseBound,
						Reason:           "Succeeded",
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceBindingOutputs{
							URI: &expectedURI,
						},
						ObservedGeneration: 2,
					},
				}

				result := controller.isAlreadyInBoundState(rb, expectedURI)
				Expect(result).To(BeTrue())
			})
		})

		Context("with different phase", func() {
			It("should return false", func() {
				expectedURI := testURI
				rb := &scorev1b1.ResourceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 2,
					},
					Status: scorev1b1.ResourceBindingStatus{
						Phase:            scorev1b1.ResourceBindingPhasePending,
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceBindingOutputs{
							URI: &expectedURI,
						},
						ObservedGeneration: 2,
					},
				}

				result := controller.isAlreadyInBoundState(rb, expectedURI)
				Expect(result).To(BeFalse())
			})
		})

		Context("with outputs not available", func() {
			It("should return false", func() {
				expectedURI := testURI
				rb := &scorev1b1.ResourceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 2,
					},
					Status: scorev1b1.ResourceBindingStatus{
						Phase:            scorev1b1.ResourceBindingPhaseBound,
						OutputsAvailable: false,
						Outputs: scorev1b1.ResourceBindingOutputs{
							URI: &expectedURI,
						},
						ObservedGeneration: 2,
					},
				}

				result := controller.isAlreadyInBoundState(rb, expectedURI)
				Expect(result).To(BeFalse())
			})
		})

		Context("with nil URI in outputs", func() {
			It("should return false", func() {
				expectedURI := testURI
				rb := &scorev1b1.ResourceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 2,
					},
					Status: scorev1b1.ResourceBindingStatus{
						Phase:            scorev1b1.ResourceBindingPhaseBound,
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceBindingOutputs{
							URI: nil,
						},
						ObservedGeneration: 2,
					},
				}

				result := controller.isAlreadyInBoundState(rb, expectedURI)
				Expect(result).To(BeFalse())
			})
		})

		Context("with different URI in outputs", func() {
			It("should return false", func() {
				expectedURI := testURI
				differentURI := "https://different.com/api"
				rb := &scorev1b1.ResourceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 2,
					},
					Status: scorev1b1.ResourceBindingStatus{
						Phase:            scorev1b1.ResourceBindingPhaseBound,
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceBindingOutputs{
							URI: &differentURI,
						},
						ObservedGeneration: 2,
					},
				}

				result := controller.isAlreadyInBoundState(rb, expectedURI)
				Expect(result).To(BeFalse())
			})
		})

		Context("with different observed generation", func() {
			It("should return false", func() {
				expectedURI := testURI
				rb := &scorev1b1.ResourceBinding{
					ObjectMeta: metav1.ObjectMeta{
						Generation: 3,
					},
					Status: scorev1b1.ResourceBindingStatus{
						Phase:            scorev1b1.ResourceBindingPhaseBound,
						OutputsAvailable: true,
						Outputs: scorev1b1.ResourceBindingOutputs{
							URI: &expectedURI,
						},
						ObservedGeneration: 2,
					},
				}

				result := controller.isAlreadyInBoundState(rb, expectedURI)
				Expect(result).To(BeFalse())
			})
		})
	})
})
