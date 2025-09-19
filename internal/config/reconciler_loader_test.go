package config

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

var _ = Describe("ReconcilerConfigLoader", func() {
	var (
		ctx           context.Context
		client        *fake.Clientset
		loader        *ReconcilerConfigLoader
		opts          ReconcilerLoaderOptions
		configMapName = "test-reconciler-config"
		namespace     = "test-namespace"
	)

	BeforeEach(func() {
		ctx = context.Background()
		client = fake.NewSimpleClientset()
		opts = ReconcilerLoaderOptions{
			ConfigMapName: configMapName,
			ConfigMapKey:  "config.yaml",
			Namespace:     namespace,
		}
		loader = NewReconcilerConfigLoader(client, opts)
	})

	AfterEach(func() {
		if loader != nil {
			_ = loader.Close()
		}
	})

	Describe("Load", func() {
		Context("when ConfigMap does not exist", func() {
			It("should return default configuration", func() {
				config, err := loader.Load(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())

				defaultConfig := DefaultReconcilerConfig()
				Expect(config.Retry.DefaultRequeueDelay).To(Equal(defaultConfig.Retry.DefaultRequeueDelay))
				Expect(config.Retry.ConflictRequeueDelay).To(Equal(defaultConfig.Retry.ConflictRequeueDelay))
			})
		})

		Context("when ConfigMap exists with reconciler config", func() {
			BeforeEach(func() {
				configMapData := `
apiVersion: score.dev/v1b1
kind: OrchestratorConfig
metadata:
  name: test-config
spec:
  profiles:
    - name: default
      backends:
        - backendId: k8s-test
          runtimeClass: kubernetes
          template:
            kind: manifests
            ref: "registry.example.com/test@sha256:abc123"
          priority: 100
          version: "1.0.0"
  defaults:
    profile: default
reconciler:
  retry:
    defaultRequeueDelay: 45s
    conflictRequeueDelay: 2s
    maxRetries: 5
    backoffMultiplier: 3.0
  timeouts:
    claimTimeout: 10m
    planTimeout: 5m
    statusTimeout: 60s
    deletionTimeout: 15m
  features:
    enableDetailedLogging: true
    enableMetrics: false
`

				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"config.yaml": configMapData,
					},
				}

				_, err := client.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should load and parse the reconciler configuration", func() {
				config, err := loader.Load(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())

				Expect(config.Retry.DefaultRequeueDelay).To(Equal(45 * time.Second))
				Expect(config.Retry.ConflictRequeueDelay).To(Equal(2 * time.Second))
				Expect(config.Retry.MaxRetries).To(Equal(5))
				Expect(config.Retry.BackoffMultiplier).To(Equal(3.0))

				Expect(config.Timeouts.ClaimTimeout).To(Equal(10 * time.Minute))
				Expect(config.Timeouts.PlanTimeout).To(Equal(5 * time.Minute))
				Expect(config.Timeouts.StatusTimeout).To(Equal(60 * time.Second))
				Expect(config.Timeouts.DeletionTimeout).To(Equal(15 * time.Minute))

				Expect(config.Features.EnableDetailedLogging).To(BeTrue())
				Expect(config.Features.EnableMetrics).To(BeFalse())
			})
		})

		Context("when ConfigMap exists without reconciler config", func() {
			BeforeEach(func() {
				configMapData := `
apiVersion: score.dev/v1b1
kind: OrchestratorConfig
metadata:
  name: test-config
spec:
  profiles:
    - name: default
      backends:
        - backendId: k8s-test
          runtimeClass: kubernetes
          template:
            kind: manifests
            ref: "registry.example.com/test@sha256:abc123"
          priority: 100
          version: "1.0.0"
  defaults:
    profile: default
`

				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"config.yaml": configMapData,
					},
				}

				_, err := client.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should return default configuration", func() {
				config, err := loader.Load(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())

				defaultConfig := DefaultReconcilerConfig()
				Expect(config.Retry.DefaultRequeueDelay).To(Equal(defaultConfig.Retry.DefaultRequeueDelay))
				Expect(config.Retry.ConflictRequeueDelay).To(Equal(defaultConfig.Retry.ConflictRequeueDelay))
			})
		})

		Context("when ConfigMap has partial reconciler config", func() {
			BeforeEach(func() {
				configMapData := `
apiVersion: score.dev/v1b1
kind: OrchestratorConfig
metadata:
  name: test-config
spec:
  profiles:
    - name: default
      backends:
        - backendId: k8s-test
          runtimeClass: kubernetes
          template:
            kind: manifests
            ref: "registry.example.com/test@sha256:abc123"
          priority: 100
          version: "1.0.0"
  defaults:
    profile: default
reconciler:
  retry:
    defaultRequeueDelay: 60s
  features:
    enableDetailedLogging: true
`

				configMap := &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Name:      configMapName,
						Namespace: namespace,
					},
					Data: map[string]string{
						"config.yaml": configMapData,
					},
				}

				_, err := client.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
				Expect(err).ToNot(HaveOccurred())
			})

			It("should merge with defaults for missing values", func() {
				config, err := loader.Load(ctx)
				Expect(err).ToNot(HaveOccurred())
				Expect(config).ToNot(BeNil())

				// Configured value
				Expect(config.Retry.DefaultRequeueDelay).To(Equal(60 * time.Second))
				Expect(config.Features.EnableDetailedLogging).To(BeTrue())

				// Default values for missing fields
				defaultConfig := DefaultReconcilerConfig()
				Expect(config.Retry.ConflictRequeueDelay).To(Equal(defaultConfig.Retry.ConflictRequeueDelay))
				Expect(config.Timeouts.ClaimTimeout).To(Equal(defaultConfig.Timeouts.ClaimTimeout))
				Expect(config.Features.EnableMetrics).To(Equal(defaultConfig.Features.EnableMetrics))
			})
		})
	})

	Describe("GetCachedConfig", func() {
		It("should return default config when no config is loaded", func() {
			config := loader.GetCachedConfig()
			Expect(config).ToNot(BeNil())

			defaultConfig := DefaultReconcilerConfig()
			Expect(config.Retry.DefaultRequeueDelay).To(Equal(defaultConfig.Retry.DefaultRequeueDelay))
		})

		It("should return cached config after loading", func() {
			configMapData := `
apiVersion: score.dev/v1b1
kind: OrchestratorConfig
metadata:
  name: test-config
spec:
  profiles:
    - name: default
      backends:
        - backendId: k8s-test
          runtimeClass: kubernetes
          template:
            kind: manifests
            ref: "registry.example.com/test@sha256:abc123"
          priority: 100
          version: "1.0.0"
  defaults:
    profile: default
reconciler:
  retry:
    defaultRequeueDelay: 90s
`

			configMap := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: namespace,
				},
				Data: map[string]string{
					"config.yaml": configMapData,
				},
			}

			_, err := client.CoreV1().ConfigMaps(namespace).Create(ctx, configMap, metav1.CreateOptions{})
			Expect(err).ToNot(HaveOccurred())

			// Load config
			_, err = loader.Load(ctx)
			Expect(err).ToNot(HaveOccurred())

			// Get cached config
			cachedConfig := loader.GetCachedConfig()
			Expect(cachedConfig).ToNot(BeNil())
			Expect(cachedConfig.Retry.DefaultRequeueDelay).To(Equal(90 * time.Second))
		})
	})

	Describe("DefaultReconcilerLoaderOptions", func() {
		It("should return sensible defaults", func() {
			opts := DefaultReconcilerLoaderOptions()

			Expect(opts.ConfigMapName).To(Equal("score-orchestrator-reconciler-config"))
			Expect(opts.ConfigMapKey).To(Equal("config.yaml"))
			Expect(opts.Namespace).To(Equal("score-system"))
		})
	})
})
