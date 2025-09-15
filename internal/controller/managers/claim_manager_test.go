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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"github.com/cappyzawa/score-orchestrator/internal/conditions"
)

func TestClaimManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ClaimManager Suite")
}

var _ = Describe("ClaimManager", func() {
	var (
		ctx          context.Context
		claimManager *ClaimManager
		fakeClient   client.Client
		scheme       *runtime.Scheme
		recorder     *record.FakeRecorder
		workload     *scorev1b1.Workload
	)

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(scorev1b1.AddToScheme(scheme)).To(Succeed())

		recorder = record.NewFakeRecorder(10)
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()
		claimManager = NewClaimManager(fakeClient, scheme, recorder)

		// Create a test workload
		workload = &scorev1b1.Workload{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-workload",
				Namespace: "default",
			},
			Spec: scorev1b1.WorkloadSpec{
				Resources: map[string]scorev1b1.ResourceSpec{
					"db": {
						Type:  "postgresql",
						Class: stringPtr("standard"),
					},
					"cache": {
						Type: "redis",
					},
				},
			},
		}
		Expect(fakeClient.Create(ctx, workload)).To(Succeed())
	})

	Describe("EnsureClaims", func() {
		It("should create ResourceClaims for all resources in the workload", func() {
			err := claimManager.EnsureClaims(ctx, workload)
			Expect(err).ToNot(HaveOccurred())

			// Verify db claim was created
			dbClaim := &scorev1b1.ResourceClaim{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      "test-workload-db",
				Namespace: "default",
			}, dbClaim)
			Expect(err).ToNot(HaveOccurred())
			Expect(dbClaim.Spec.Type).To(Equal("postgresql"))
			Expect(dbClaim.Spec.Class).ToNot(BeNil())
			Expect(*dbClaim.Spec.Class).To(Equal("standard"))
			Expect(dbClaim.Spec.Key).To(Equal("db"))

			// Verify cache claim was created
			cacheClaim := &scorev1b1.ResourceClaim{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      "test-workload-cache",
				Namespace: "default",
			}, cacheClaim)
			Expect(err).ToNot(HaveOccurred())
			Expect(cacheClaim.Spec.Type).To(Equal("redis"))
			Expect(cacheClaim.Spec.Class).To(BeNil())
			Expect(cacheClaim.Spec.Key).To(Equal("cache"))
		})

		It("should update existing claims when spec changes", func() {
			// Create initial claim
			err := claimManager.EnsureClaims(ctx, workload)
			Expect(err).ToNot(HaveOccurred())

			// Update workload spec
			dbResource := workload.Spec.Resources["db"]
			dbResource.Class = stringPtr("premium")
			workload.Spec.Resources["db"] = dbResource
			Expect(fakeClient.Update(ctx, workload)).To(Succeed())

			// Ensure claims again
			err = claimManager.EnsureClaims(ctx, workload)
			Expect(err).ToNot(HaveOccurred())

			// Verify claim was updated
			dbClaim := &scorev1b1.ResourceClaim{}
			err = fakeClient.Get(ctx, types.NamespacedName{
				Name:      "test-workload-db",
				Namespace: "default",
			}, dbClaim)
			Expect(err).ToNot(HaveOccurred())
			Expect(*dbClaim.Spec.Class).To(Equal("premium"))
		})
	})

	Describe("GetClaims", func() {
		It("should retrieve claims using label selector", func() {
			// Create claims first
			err := claimManager.EnsureClaims(ctx, workload)
			Expect(err).ToNot(HaveOccurred())

			// Get claims
			claims, err := claimManager.GetClaims(ctx, workload)
			Expect(err).ToNot(HaveOccurred())
			Expect(claims).To(HaveLen(2))

			// Verify claims contain expected keys
			keys := make([]string, len(claims))
			for i, claim := range claims {
				keys[i] = claim.Spec.Key
			}
			Expect(keys).To(ContainElements("db", "cache"))
		})

		It("should return empty slice when no claims exist", func() {
			claims, err := claimManager.GetClaims(ctx, workload)
			Expect(err).ToNot(HaveOccurred())
			Expect(claims).To(BeEmpty())
		})
	})

	Describe("AggregateStatus", func() {
		It("should return not ready when no claims", func() {
			agg := claimManager.AggregateStatus([]scorev1b1.ResourceClaim{})
			Expect(agg.Ready).To(BeFalse())
			Expect(agg.Reason).To(Equal(conditions.ReasonClaimPending))
			Expect(agg.Message).To(Equal(conditions.MessageNoClaimsFound))
			Expect(agg.Claims).To(BeEmpty())
		})

		It("should return ready when all claims are bound with outputs", func() {
			claims := []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "db"},
					Status: scorev1b1.ResourceClaimStatus{
						Phase:            scorev1b1.ResourceClaimPhaseBound,
						OutputsAvailable: true,
						Reason:           conditions.ReasonSucceeded,
						Message:          "Database ready",
					},
				},
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "cache"},
					Status: scorev1b1.ResourceClaimStatus{
						Phase:            scorev1b1.ResourceClaimPhaseBound,
						OutputsAvailable: true,
						Reason:           conditions.ReasonSucceeded,
						Message:          "Cache ready",
					},
				},
			}

			agg := claimManager.AggregateStatus(claims)
			Expect(agg.Ready).To(BeTrue())
			Expect(agg.Reason).To(Equal(conditions.ReasonSucceeded))
			Expect(agg.Message).To(Equal(conditions.MessageAllClaimsReady))
			Expect(agg.Claims).To(HaveLen(2))
		})

		It("should return not ready when some claims are pending", func() {
			claims := []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "db"},
					Status: scorev1b1.ResourceClaimStatus{
						Phase:            scorev1b1.ResourceClaimPhaseBound,
						OutputsAvailable: true,
					},
				},
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "cache"},
					Status: scorev1b1.ResourceClaimStatus{
						Phase: scorev1b1.ResourceClaimPhasePending,
					},
				},
			}

			agg := claimManager.AggregateStatus(claims)
			Expect(agg.Ready).To(BeFalse())
			Expect(agg.Reason).To(Equal(conditions.ReasonClaimPending))
			Expect(agg.Message).To(Equal(conditions.MessageClaimsProvisioning))
		})

		It("should return not ready when any claim has failed", func() {
			claims := []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "db"},
					Status: scorev1b1.ResourceClaimStatus{
						Phase:            scorev1b1.ResourceClaimPhaseBound,
						OutputsAvailable: true,
					},
				},
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "cache"},
					Status: scorev1b1.ResourceClaimStatus{
						Phase: scorev1b1.ResourceClaimPhaseFailed,
					},
				},
			}

			agg := claimManager.AggregateStatus(claims)
			Expect(agg.Ready).To(BeFalse())
			Expect(agg.Reason).To(Equal(conditions.ReasonClaimFailed))
			Expect(agg.Message).To(Equal(conditions.MessageClaimsFailed))
		})

		It("should handle empty phase as pending", func() {
			claims := []scorev1b1.ResourceClaim{
				{
					Spec: scorev1b1.ResourceClaimSpec{Key: "db"},
					Status: scorev1b1.ResourceClaimStatus{
						Phase: "", // Empty phase should be treated as pending
					},
				},
			}

			agg := claimManager.AggregateStatus(claims)
			Expect(agg.Claims[0].Phase).To(Equal(scorev1b1.ResourceClaimPhasePending))
		})
	})
})

// Helper function to create string pointers
func stringPtr(s string) *string {
	return &s
}
