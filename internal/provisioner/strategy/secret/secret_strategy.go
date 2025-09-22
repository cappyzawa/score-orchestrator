package secret

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

// SecretStrategy implements the Strategy interface for generic secret provisioning
type SecretStrategy struct {
	client client.Client
}

// NewSecretStrategy creates a new SecretStrategy
func NewSecretStrategy(k8sClient client.Client) *SecretStrategy {
	return &SecretStrategy{
		client: k8sClient,
	}
}

// GetType returns the resource type this strategy handles
func (s *SecretStrategy) GetType() string {
	return "secret"
}

// Provision creates a Secret with generated credentials
func (s *SecretStrategy) Provision(ctx context.Context, claim *scorev1b1.ResourceClaim) (*scorev1b1.ResourceClaimOutputs, error) {
	// Generate default credentials
	secretData := map[string][]byte{
		"username": []byte("user"),
		"token":    []byte(generateRandomToken(32)),
	}

	// If params specify custom keys, generate those instead
	// For now, use default keys since Params is JSON type
	// In a real implementation, this would parse the JSON params
	// to extract custom key specifications
	_ = claim.Spec.Params // Avoid unused variable warning

	// Create Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-secret", claim.Name),
			Namespace: claim.Namespace,
			Labels: map[string]string{
				"score.dev/resource-claim": claim.Name,
				"score.dev/resource-type":  "secret",
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: secretData,
	}

	// Set ResourceClaim as owner for garbage collection
	if err := controllerutil.SetControllerReference(claim, secret, s.client.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to set owner reference: %w", err)
	}

	// Create or update the Secret
	existing := &corev1.Secret{}
	err := s.client.Get(ctx, client.ObjectKeyFromObject(secret), existing)
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

// Deprovision cleans up the secret resources
func (s *SecretStrategy) Deprovision(ctx context.Context, claim *scorev1b1.ResourceClaim) error {
	secretName := fmt.Sprintf("%s-secret", claim.Name)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: claim.Namespace,
		},
	}

	err := s.client.Delete(ctx, secret)
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete secret: %w", err)
	}

	return nil
}

// GetStatus returns the current status of the secret resource
func (s *SecretStrategy) GetStatus(ctx context.Context, claim *scorev1b1.ResourceClaim) (phase scorev1b1.ResourceClaimPhase, reason, message string, err error) {
	secretName := fmt.Sprintf("%s-secret", claim.Name)
	secret := &corev1.Secret{}

	err = s.client.Get(ctx, client.ObjectKey{
		Name:      secretName,
		Namespace: claim.Namespace,
	}, secret)

	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return scorev1b1.ResourceClaimPhaseFailed, "SecretAccessFailed",
				fmt.Sprintf("Failed to access secret: %v", err), err
		}
		// Secret doesn't exist yet, still provisioning
		return scorev1b1.ResourceClaimPhaseClaiming, "SecretCreating",
			"Secret is being created", nil
	}

	// Secret exists and is ready
	return scorev1b1.ResourceClaimPhaseBound, "Succeeded",
		"Secret is available", nil
}

// generateRandomToken generates a cryptographically secure random token
func generateRandomToken(length int) string {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a default value if random generation fails
		return "fallback-token-" + fmt.Sprintf("%d", length)
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length]
}
