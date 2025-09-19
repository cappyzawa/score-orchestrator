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

package provisioner

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/client"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// ExternalApiStrategy implements the external API provisioning strategy
type ExternalApiStrategy struct{}

// Provision implements the Strategy interface for external API provisioning
// NOTE: This is a stub implementation for Core Controller Implementation phase
// Full external API integration will be implemented in Phase 2
func (s *ExternalApiStrategy) Provision(ctx context.Context, c client.Client, claim *scorev1b1.ResourceClaim, config *scorev1b1.ProvisionerConfig, templateCtx *TemplateContext) (*ProvisioningResult, error) {
	// For Core Controller Implementation, external API strategy is not yet implemented
	return &ProvisioningResult{
		Phase:   scorev1b1.ResourceClaimPhaseFailed,
		Reason:  "NotImplemented",
		Message: fmt.Sprintf("External API strategy is not implemented in Core Controller phase (will be available in Phase 2): %v", ErrStrategyNotImplemented),
	}, nil
}
