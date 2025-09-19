package provisioner

import (
	"fmt"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// OutputManager handles ResourceClaim output validation and management
type OutputManager struct{}

// NewOutputManager creates a new OutputManager
func NewOutputManager() *OutputManager {
	return &OutputManager{}
}

// ValidateOutputs validates that outputs comply with the CEL constraint:
// at least one of secretRef, configMapRef, uri, image, or cert must be set
func (om *OutputManager) ValidateOutputs(outputs *scorev1b1.ResourceClaimOutputs) error {
	if outputs == nil {
		return fmt.Errorf("outputs cannot be nil")
	}

	// Check if at least one output field is set (CEL constraint)
	hasSecretRef := outputs.SecretRef != nil && outputs.SecretRef.Name != ""
	hasConfigMapRef := outputs.ConfigMapRef != nil && outputs.ConfigMapRef.Name != ""
	hasURI := outputs.URI != nil && *outputs.URI != ""
	hasImage := outputs.Image != nil && *outputs.Image != ""
	hasCert := outputs.Cert != nil && (outputs.Cert.SecretName != nil || len(outputs.Cert.Data) > 0)

	if !hasSecretRef && !hasConfigMapRef && !hasURI && !hasImage && !hasCert {
		return fmt.Errorf("at least one output field (secretRef, configMapRef, uri, image, or cert) must be set")
	}

	return nil
}

// SetOutputsAvailable updates the outputsAvailable field based on outputs validation
func (om *OutputManager) SetOutputsAvailable(claim *scorev1b1.ResourceClaim) {
	err := om.ValidateOutputs(claim.Status.Outputs)
	claim.Status.OutputsAvailable = err == nil
}

// CreateSecretRefOutput creates an output with a secret reference
func (om *OutputManager) CreateSecretRefOutput(secretName string) *scorev1b1.ResourceClaimOutputs {
	return &scorev1b1.ResourceClaimOutputs{
		SecretRef: &scorev1b1.LocalObjectReference{
			Name: secretName,
		},
	}
}

// CreateConfigMapRefOutput creates an output with a configmap reference
func (om *OutputManager) CreateConfigMapRefOutput(configMapName string) *scorev1b1.ResourceClaimOutputs {
	return &scorev1b1.ResourceClaimOutputs{
		ConfigMapRef: &scorev1b1.LocalObjectReference{
			Name: configMapName,
		},
	}
}

// CreateURIOutput creates an output with a URI
func (om *OutputManager) CreateURIOutput(uri string) *scorev1b1.ResourceClaimOutputs {
	return &scorev1b1.ResourceClaimOutputs{
		URI: &uri,
	}
}

// CreateImageOutput creates an output with an image reference
func (om *OutputManager) CreateImageOutput(image string) *scorev1b1.ResourceClaimOutputs {
	return &scorev1b1.ResourceClaimOutputs{
		Image: &image,
	}
}

// MergeOutputs merges multiple output sources into a single output
func (om *OutputManager) MergeOutputs(outputs ...*scorev1b1.ResourceClaimOutputs) *scorev1b1.ResourceClaimOutputs {
	result := &scorev1b1.ResourceClaimOutputs{}

	for _, output := range outputs {
		if output == nil {
			continue
		}

		if output.SecretRef != nil && result.SecretRef == nil {
			result.SecretRef = output.SecretRef
		}
		if output.ConfigMapRef != nil && result.ConfigMapRef == nil {
			result.ConfigMapRef = output.ConfigMapRef
		}
		if output.URI != nil && result.URI == nil {
			result.URI = output.URI
		}
		if output.Image != nil && result.Image == nil {
			result.Image = output.Image
		}
		if output.Cert != nil && result.Cert == nil {
			result.Cert = output.Cert
		}
	}

	return result
}
