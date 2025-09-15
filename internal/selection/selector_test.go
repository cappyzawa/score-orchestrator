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

package selection

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

func TestProfileSelector(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ProfileSelector Suite")
}

var _ = Describe("ProfileSelector", func() {
	var (
		scheme *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(corev1.AddToScheme(scheme)).ToNot(HaveOccurred())
		Expect(scorev1b1.AddToScheme(scheme)).ToNot(HaveOccurred())
	})

	Describe("SelectBackend", func() {
		Context("when user hint profile is provided", func() {
			It("should select the specified profile backend", func() {
				config := &scorev1b1.OrchestratorConfig{
					Spec: scorev1b1.OrchestratorConfigSpec{
						Profiles: []scorev1b1.ProfileSpec{
							{
								Name: "web-service",
								Backends: []scorev1b1.BackendSpec{
									{
										BackendId:    "k8s-web",
										RuntimeClass: "kubernetes",
										Priority:     100,
										Version:      "1.0.0",
									},
								},
							},
						},
					},
				}

				workload := &scorev1b1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workload",
						Namespace: "default",
						Annotations: map[string]string{
							"score.dev/profile": "web-service",
						},
					},
				}

				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "default"},
				}

				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespace).Build()
				selector := NewProfileSelector(config, client)

				result, err := selector.SelectBackend(context.Background(), workload)

				Expect(err).ToNot(HaveOccurred())
				Expect(result).ToNot(BeNil())
				Expect(result.BackendID).To(Equal("k8s-web"))
				Expect(result.RuntimeClass).To(Equal("kubernetes"))
				Expect(result.Priority).To(Equal(100))
				Expect(result.Version).To(Equal("1.0.0"))
			})
		})

		Context("when invalid user hint is provided", func() {
			It("should fail", func() {
				config := &scorev1b1.OrchestratorConfig{
					Spec: scorev1b1.OrchestratorConfigSpec{
						Profiles: []scorev1b1.ProfileSpec{
							{
								Name: "web-service",
								Backends: []scorev1b1.BackendSpec{
									{
										BackendId:    "k8s-web",
										RuntimeClass: "kubernetes",
										Priority:     100,
										Version:      "1.0.0",
									},
								},
							},
						},
					},
				}

				workload := &scorev1b1.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-workload",
						Namespace: "default",
						Annotations: map[string]string{
							"score.dev/profile": "nonexistent-profile",
						},
					},
				}

				namespace := &corev1.Namespace{
					ObjectMeta: metav1.ObjectMeta{Name: "default"},
				}

				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(namespace).Build()
				selector := NewProfileSelector(config, client)

				result, err := selector.SelectBackend(context.Background(), workload)

				Expect(err).To(HaveOccurred())
				Expect(result).To(BeNil())
			})
		})
	})
})
