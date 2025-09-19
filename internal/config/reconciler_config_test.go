package config

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestReconcilerConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ReconcilerConfig Suite")
}

var _ = Describe("ReconcilerConfig", func() {
	Describe("DefaultReconcilerConfig", func() {
		It("should return config with sensible defaults", func() {
			config := DefaultReconcilerConfig()

			Expect(config).ToNot(BeNil())
			Expect(config.Retry.DefaultRequeueDelay).To(Equal(30 * time.Second))
			Expect(config.Retry.ConflictRequeueDelay).To(Equal(1 * time.Second))
			Expect(config.Retry.MaxRetries).To(Equal(3))
			Expect(config.Retry.BackoffMultiplier).To(Equal(2.0))

			Expect(config.Timeouts.ClaimTimeout).To(Equal(5 * time.Minute))
			Expect(config.Timeouts.PlanTimeout).To(Equal(3 * time.Minute))
			Expect(config.Timeouts.StatusTimeout).To(Equal(30 * time.Second))
			Expect(config.Timeouts.DeletionTimeout).To(Equal(10 * time.Minute))

			Expect(config.Features.EnableDetailedLogging).To(BeFalse())
			Expect(config.Features.EnableMetrics).To(BeTrue())
			Expect(config.Features.EnableTracing).To(BeFalse())
			Expect(config.Features.EnableExperimentalFeatures).To(BeFalse())
		})
	})

	Describe("Validate", func() {
		var config *ReconcilerConfig

		BeforeEach(func() {
			config = DefaultReconcilerConfig()
		})

		It("should pass validation with default values", func() {
			err := config.Validate()
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fix invalid DefaultRequeueDelay", func() {
			config.Retry.DefaultRequeueDelay = -1 * time.Second
			err := config.Validate()
			Expect(err).ToNot(HaveOccurred())
			Expect(config.Retry.DefaultRequeueDelay).To(Equal(30 * time.Second))
		})

		It("should fix invalid ConflictRequeueDelay", func() {
			config.Retry.ConflictRequeueDelay = 0
			err := config.Validate()
			Expect(err).ToNot(HaveOccurred())
			Expect(config.Retry.ConflictRequeueDelay).To(Equal(1 * time.Second))
		})

		It("should fix invalid MaxRetries", func() {
			config.Retry.MaxRetries = -1
			err := config.Validate()
			Expect(err).ToNot(HaveOccurred())
			Expect(config.Retry.MaxRetries).To(Equal(3))
		})

		It("should fix invalid BackoffMultiplier", func() {
			config.Retry.BackoffMultiplier = 0.5
			err := config.Validate()
			Expect(err).ToNot(HaveOccurred())
			Expect(config.Retry.BackoffMultiplier).To(Equal(2.0))
		})

		It("should fix invalid ClaimTimeout", func() {
			config.Timeouts.ClaimTimeout = -1 * time.Minute
			err := config.Validate()
			Expect(err).ToNot(HaveOccurred())
			Expect(config.Timeouts.ClaimTimeout).To(Equal(5 * time.Minute))
		})

		It("should fix invalid PlanTimeout", func() {
			config.Timeouts.PlanTimeout = 0
			err := config.Validate()
			Expect(err).ToNot(HaveOccurred())
			Expect(config.Timeouts.PlanTimeout).To(Equal(3 * time.Minute))
		})

		It("should fix invalid StatusTimeout", func() {
			config.Timeouts.StatusTimeout = -30 * time.Second
			err := config.Validate()
			Expect(err).ToNot(HaveOccurred())
			Expect(config.Timeouts.StatusTimeout).To(Equal(30 * time.Second))
		})

		It("should fix invalid DeletionTimeout", func() {
			config.Timeouts.DeletionTimeout = 0
			err := config.Validate()
			Expect(err).ToNot(HaveOccurred())
			Expect(config.Timeouts.DeletionTimeout).To(Equal(10 * time.Minute))
		})
	})
})
