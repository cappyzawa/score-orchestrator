package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// PostgresStrategy implements the Strategy interface for PostgreSQL provisioning
type PostgresStrategy struct {
	client client.Client
}

// NewPostgresStrategy creates a new PostgresStrategy
func NewPostgresStrategy(k8sClient client.Client) *PostgresStrategy {
	return &PostgresStrategy{
		client: k8sClient,
	}
}

// GetType returns the resource type this strategy handles
func (s *PostgresStrategy) GetType() string {
	return "postgres"
}

// Provision creates PostgreSQL development resources: StatefulSet, Service, and Secret
func (s *PostgresStrategy) Provision(ctx context.Context, claim *scorev1b1.ResourceClaim) (*scorev1b1.ResourceClaimOutputs, error) {
	// Generate database credentials
	username := "postgres"
	password, err := generateRandomPassword(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate password: %w", err)
	}

	host := fmt.Sprintf("%s-postgres-service", claim.Name)
	port := "5432"

	// Create StatefulSet for PostgreSQL
	if err := s.createStatefulSet(ctx, claim, username, password); err != nil {
		return nil, fmt.Errorf("failed to create postgres statefulset: %w", err)
	}

	// Create Service for PostgreSQL
	if err := s.createService(ctx, claim); err != nil {
		return nil, fmt.Errorf("failed to create postgres service: %w", err)
	}

	// Create Secret with database credentials
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-postgres-secret", claim.Name),
			Namespace: claim.Namespace,
			Labels: map[string]string{
				"score.dev/resource-claim": claim.Name,
				"score.dev/resource-type":  "postgres",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"username": []byte(username),
			"password": []byte(password),
			"host":     []byte(host),
			"port":     []byte(port),
			"database": []byte("postgres"),
			// Connection string for convenience
			"uri": []byte(fmt.Sprintf("postgresql://%s:%s@%s:%s/postgres", username, password, host, port)),
		},
	}

	// Set ResourceClaim as owner for garbage collection
	if err := controllerutil.SetControllerReference(claim, secret, s.client.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create or update the Secret
	existing := &corev1.Secret{}
	err = s.client.Get(ctx, client.ObjectKeyFromObject(secret), existing)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return nil, fmt.Errorf("failed to check existing secret: %w", err)
		}
		// Secret doesn't exist, create it
		if err := s.client.Create(ctx, secret); err != nil {
			return nil, fmt.Errorf("failed to create secret: %w", err)
		}
	}

	// Return outputs pointing to the created Secret
	outputs := &scorev1b1.ResourceClaimOutputs{
		SecretRef: &scorev1b1.LocalObjectReference{
			Name: secret.Name,
		},
	}

	return outputs, nil
}

// Deprovision cleans up all PostgreSQL resources: StatefulSet, Service, and Secret
func (s *PostgresStrategy) Deprovision(ctx context.Context, claim *scorev1b1.ResourceClaim) error {
	// Delete StatefulSet
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-postgres", claim.Name),
			Namespace: claim.Namespace,
		},
	}
	err := s.client.Delete(ctx, statefulSet)
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete postgres statefulset: %w", err)
	}

	// Delete Service
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-postgres-service", claim.Name),
			Namespace: claim.Namespace,
		},
	}
	err = s.client.Delete(ctx, service)
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete postgres service: %w", err)
	}

	// Delete Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-postgres-secret", claim.Name),
			Namespace: claim.Namespace,
		},
	}
	err = s.client.Delete(ctx, secret)
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete postgres secret: %w", err)
	}

	return nil
}

// GetStatus returns the current status of the PostgreSQL resource
func (s *PostgresStrategy) GetStatus(ctx context.Context, claim *scorev1b1.ResourceClaim) (phase scorev1b1.ResourceClaimPhase, reason, message string, err error) {
	// Check Secret
	secretName := fmt.Sprintf("%s-postgres-secret", claim.Name)
	secret := &corev1.Secret{}

	err = s.client.Get(ctx, client.ObjectKey{
		Name:      secretName,
		Namespace: claim.Namespace,
	}, secret)

	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return scorev1b1.ResourceClaimPhaseFailed, "SecretAccessFailed",
				fmt.Sprintf("Failed to access postgres secret: %v", err), err
		}
		// Secret doesn't exist yet, still provisioning
		return scorev1b1.ResourceClaimPhaseClaiming, "ResourcesCreating",
			"PostgreSQL resources are being created", nil
	}

	// Check StatefulSet
	statefulSetName := fmt.Sprintf("%s-postgres", claim.Name)
	statefulSet := &appsv1.StatefulSet{}

	err = s.client.Get(ctx, client.ObjectKey{
		Name:      statefulSetName,
		Namespace: claim.Namespace,
	}, statefulSet)

	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return scorev1b1.ResourceClaimPhaseFailed, "StatefulSetAccessFailed",
				fmt.Sprintf("Failed to access postgres statefulset: %v", err), err
		}
		// StatefulSet doesn't exist yet, still provisioning
		return scorev1b1.ResourceClaimPhaseClaiming, "StatefulSetCreating",
			"PostgreSQL StatefulSet is being created", nil
	}

	// Check if StatefulSet is ready
	if statefulSet.Status.ReadyReplicas < *statefulSet.Spec.Replicas {
		return scorev1b1.ResourceClaimPhaseClaiming, "StatefulSetNotReady",
			"PostgreSQL StatefulSet is not ready yet", nil
	}

	// Check Service
	serviceName := fmt.Sprintf("%s-postgres-service", claim.Name)
	service := &corev1.Service{}

	err = s.client.Get(ctx, client.ObjectKey{
		Name:      serviceName,
		Namespace: claim.Namespace,
	}, service)

	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return scorev1b1.ResourceClaimPhaseFailed, "ServiceAccessFailed",
				fmt.Sprintf("Failed to access postgres service: %v", err), err
		}
		// Service doesn't exist yet, still provisioning
		return scorev1b1.ResourceClaimPhaseClaiming, "ServiceCreating",
			"PostgreSQL Service is being created", nil
	}

	// All resources exist and StatefulSet is ready
	return scorev1b1.ResourceClaimPhaseBound, "Succeeded",
		"PostgreSQL instance is ready and available", nil
}

// generateRandomPassword generates a cryptographically secure random password
func generateRandomPassword(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Use base64 encoding for a readable password
	password := base64.URLEncoding.EncodeToString(bytes)

	// Trim to desired length
	if len(password) > length {
		password = password[:length]
	}

	return password, nil
}

// createStatefulSet creates a PostgreSQL StatefulSet for development use
func (s *PostgresStrategy) createStatefulSet(ctx context.Context, claim *scorev1b1.ResourceClaim, username, password string) error {
	statefulSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-postgres", claim.Name),
			Namespace: claim.Namespace,
			Labels: map[string]string{
				"score.dev/resource-claim": claim.Name,
				"score.dev/resource-type":  "postgres",
				"app":                      fmt.Sprintf("%s-postgres", claim.Name),
			},
		},
		Spec: appsv1.StatefulSetSpec{
			ServiceName: fmt.Sprintf("%s-postgres-service", claim.Name),
			Replicas:    int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": fmt.Sprintf("%s-postgres", claim.Name),
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": fmt.Sprintf("%s-postgres", claim.Name),
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsUser:    int64Ptr(999), // postgres user
						RunAsGroup:   int64Ptr(999), // postgres group
						FSGroup:      int64Ptr(999), // Set ownership for volumes
						RunAsNonRoot: boolPtr(true),
					},
					Containers: []corev1.Container{
						{
							Name:  "postgres",
							Image: "postgres:13",
							Env: []corev1.EnvVar{
								{
									Name:  "POSTGRES_USER",
									Value: username,
								},
								{
									Name:  "POSTGRES_PASSWORD",
									Value: password,
								},
								{
									Name:  "POSTGRES_DB",
									Value: "postgres",
								},
								{
									Name:  "PGDATA",
									Value: "/var/lib/postgresql/data/pgdata",
								},
							},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 5432,
									Name:          "postgres",
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "postgres-data",
									MountPath: "/var/lib/postgresql/data",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("128Mi"),
									corev1.ResourceCPU:    resource.MustParse("100m"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("512Mi"),
									corev1.ResourceCPU:    resource.MustParse("500m"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"pg_isready",
											"-U", username,
											"-d", "postgres",
										},
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"pg_isready",
											"-U", username,
											"-d", "postgres",
										},
									},
								},
								InitialDelaySeconds: 60,
								PeriodSeconds:       30,
								TimeoutSeconds:      5,
								FailureThreshold:    3,
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsUser:                int64Ptr(999),
								RunAsGroup:               int64Ptr(999),
								RunAsNonRoot:             boolPtr(true),
								AllowPrivilegeEscalation: boolPtr(false),
								ReadOnlyRootFilesystem:   boolPtr(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "postgres-data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse("1Gi"),
							},
						},
					},
				},
			},
		},
	}

	// Set ResourceClaim as owner for garbage collection
	if err := controllerutil.SetControllerReference(claim, statefulSet, s.client.Scheme()); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create or update the StatefulSet
	existing := &appsv1.StatefulSet{}
	err := s.client.Get(ctx, client.ObjectKeyFromObject(statefulSet), existing)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to check existing statefulset: %w", err)
		}
		// StatefulSet doesn't exist, create it
		if err := s.client.Create(ctx, statefulSet); err != nil {
			return fmt.Errorf("failed to create statefulset: %w", err)
		}
	}

	return nil
}

// createService creates a ClusterIP service for PostgreSQL
func (s *PostgresStrategy) createService(ctx context.Context, claim *scorev1b1.ResourceClaim) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-postgres-service", claim.Name),
			Namespace: claim.Namespace,
			Labels: map[string]string{
				"score.dev/resource-claim": claim.Name,
				"score.dev/resource-type":  "postgres",
				"app":                      fmt.Sprintf("%s-postgres", claim.Name),
			},
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "postgres",
					Port:       5432,
					TargetPort: intstr.FromInt(5432),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Selector: map[string]string{
				"app": fmt.Sprintf("%s-postgres", claim.Name),
			},
		},
	}

	// Set ResourceClaim as owner for garbage collection
	if err := controllerutil.SetControllerReference(claim, service, s.client.Scheme()); err != nil {
		return fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create or update the Service
	existing := &corev1.Service{}
	err := s.client.Get(ctx, client.ObjectKeyFromObject(service), existing)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("failed to check existing service: %w", err)
		}
		// Service doesn't exist, create it
		if err := s.client.Create(ctx, service); err != nil {
			return fmt.Errorf("failed to create service: %w", err)
		}
	}

	return nil
}

// Helper functions for pointer values
func int32Ptr(i int32) *int32 {
	return &i
}

func int64Ptr(i int64) *int64 {
	return &i
}

func boolPtr(b bool) *bool {
	return &b
}
