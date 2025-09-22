package postgres

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

// Provision creates a Secret with PostgreSQL credentials and connection details
func (s *PostgresStrategy) Provision(ctx context.Context, claim *scorev1b1.ResourceClaim) (*scorev1b1.ResourceClaimOutputs, error) {
	// Generate database credentials
	username := "postgres"
	password, err := generateRandomPassword(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate password: %w", err)
	}

	// For Phase 1, use mock values for host and port
	// In later phases, this could create actual RDS/CloudSQL resources
	host := fmt.Sprintf("%s-postgres-service", claim.Name)
	port := "5432"

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

// Deprovision cleans up the PostgreSQL resources
func (s *PostgresStrategy) Deprovision(ctx context.Context, claim *scorev1b1.ResourceClaim) error {
	secretName := fmt.Sprintf("%s-postgres-secret", claim.Name)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: claim.Namespace,
		},
	}

	err := s.client.Delete(ctx, secret)
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete postgres secret: %w", err)
	}

	return nil
}

// GetStatus returns the current status of the PostgreSQL resource
func (s *PostgresStrategy) GetStatus(ctx context.Context, claim *scorev1b1.ResourceClaim) (phase scorev1b1.ResourceClaimPhase, reason, message string, err error) {
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
		return scorev1b1.ResourceClaimPhaseClaiming, "SecretCreating",
			"PostgreSQL secret is being created", nil
	}

	// Secret exists and is ready
	return scorev1b1.ResourceClaimPhaseBound, "Succeeded",
		"PostgreSQL credentials are available", nil
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
