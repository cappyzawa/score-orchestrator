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
	"fmt"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// SelectedBackend represents the result of backend selection
type SelectedBackend struct {
	BackendID    string
	RuntimeClass string
	Template     scorev1b1.TemplateSpec
	Priority     int
	Version      string
}

// ProfileSelector interface defines the contract for profile and backend selection
type ProfileSelector interface {
	// SelectBackend selects the appropriate backend for a workload based on the
	// deterministic selection pipeline specified in the orchestrator config spec
	SelectBackend(ctx context.Context, workload *scorev1b1.Workload) (*SelectedBackend, error)
}

// profileSelector implements ProfileSelector interface
type profileSelector struct {
	config *scorev1b1.OrchestratorConfig
	client client.Client
}

// NewProfileSelector creates a new ProfileSelector instance
func NewProfileSelector(config *scorev1b1.OrchestratorConfig, k8sClient client.Client) ProfileSelector {
	return &profileSelector{
		config: config,
		client: k8sClient,
	}
}

// SelectBackend implements the deterministic selection pipeline:
// 1. Profile Selection (user hint → auto-derive → selectors → global default)
// 2. Backend Filtering (selectors, features, constraints, admission)
// 3. Backend Selection (deterministic sorting by priority → version → backendId)
func (s *profileSelector) SelectBackend(ctx context.Context, workload *scorev1b1.Workload) (*SelectedBackend, error) {
	// 1. Profile Selection
	profileName, err := s.selectProfile(ctx, workload)
	if err != nil {
		return nil, fmt.Errorf("profile selection failed: %w", err)
	}

	// Find the selected profile
	var selectedProfile *scorev1b1.ProfileSpec
	for _, profile := range s.config.Spec.Profiles {
		if profile.Name == profileName {
			selectedProfile = &profile
			break
		}
	}

	if selectedProfile == nil {
		return nil, fmt.Errorf("profile %q not found in configuration", profileName)
	}

	// 2. Backend Filtering
	candidates, err := s.filterBackends(ctx, workload, selectedProfile.Backends)
	if err != nil {
		return nil, fmt.Errorf("backend filtering failed: %w", err)
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no suitable backend candidates found for profile %q", profileName)
	}

	// 3. Backend Selection
	selectedBackend := s.selectBackend(candidates)

	return &SelectedBackend{
		BackendID:    selectedBackend.BackendId,
		RuntimeClass: selectedBackend.RuntimeClass,
		Template:     selectedBackend.Template,
		Priority:     selectedBackend.Priority,
		Version:      selectedBackend.Version,
	}, nil
}

// selectProfile implements the profile selection pipeline
func (s *profileSelector) selectProfile(ctx context.Context, workload *scorev1b1.Workload) (string, error) {
	// 1. User hint evaluation: score.dev/profile annotation on Workload
	if profileHint, exists := workload.Annotations["score.dev/profile"]; exists && profileHint != "" {
		// Validate that the hinted profile exists
		for _, profile := range s.config.Spec.Profiles {
			if profile.Name == profileHint {
				return profileHint, nil
			}
		}
		// Profile hint is invalid - this should result in SpecInvalid
		return "", fmt.Errorf("hinted profile %q does not exist", profileHint)
	}

	// 2. Auto-derivation: Profile inferred from Workload characteristics
	if derivedProfile := s.deriveProfileFromWorkload(workload); derivedProfile != "" {
		return derivedProfile, nil
	}

	// 3. Selector matching: Apply defaults.selectors[] based on namespace/labels
	selectedProfile, err := s.selectProfileFromSelectors(ctx, workload)
	if err != nil {
		return "", fmt.Errorf("selector matching failed: %w", err)
	}
	if selectedProfile != "" {
		return selectedProfile, nil
	}

	// 4. Global fallback: Use defaults.profile as final fallback
	if s.config.Spec.Defaults.Profile != "" {
		return s.config.Spec.Defaults.Profile, nil
	}

	return "", fmt.Errorf("no profile could be determined and no default profile is configured")
}

// deriveProfileFromWorkload derives profile from workload characteristics
func (s *profileSelector) deriveProfileFromWorkload(workload *scorev1b1.Workload) string {
	// Auto-derivation rules (basic implementation):
	// - If workload has service ports: likely web-service
	// - If workload has no service: likely batch-job

	if workload.Spec.Service != nil && len(workload.Spec.Service.Ports) > 0 {
		// Look for web-service profile
		for _, profile := range s.config.Spec.Profiles {
			if strings.Contains(strings.ToLower(profile.Name), "web") ||
				strings.Contains(strings.ToLower(profile.Name), "service") {
				return profile.Name
			}
		}
	} else {
		// Look for batch-job profile
		for _, profile := range s.config.Spec.Profiles {
			if strings.Contains(strings.ToLower(profile.Name), "batch") ||
				strings.Contains(strings.ToLower(profile.Name), "job") {
				return profile.Name
			}
		}
	}

	return ""
}

// selectProfileFromSelectors evaluates defaults.selectors[] in document order
func (s *profileSelector) selectProfileFromSelectors(ctx context.Context, workload *scorev1b1.Workload) (string, error) {
	// Get namespace labels
	namespace := &corev1.Namespace{}
	if err := s.client.Get(ctx, client.ObjectKey{Name: workload.Namespace}, namespace); err != nil {
		return "", fmt.Errorf("failed to get namespace %q: %w", workload.Namespace, err)
	}

	// Combine labels: Workload ∪ Namespace (Workload takes precedence)
	combinedLabels := combineLabels(workload.Labels, namespace.Labels)

	// Evaluate selectors in document order - first match wins
	for _, selector := range s.config.Spec.Defaults.Selectors {
		if s.selectorMatches(selector, combinedLabels) && selector.Profile != "" {
			return selector.Profile, nil
		}
	}

	return "", nil
}

// selectorMatches checks if a selector matches the given labels
func (s *profileSelector) selectorMatches(selector scorev1b1.SelectorSpec, targetLabels map[string]string) bool {
	// Convert to metav1.LabelSelector for standard matching logic
	labelSelector := &metav1.LabelSelector{
		MatchLabels:      selector.MatchLabels,
		MatchExpressions: selector.MatchExpressions,
	}

	parsedSelector, err := metav1.LabelSelectorAsSelector(labelSelector)
	if err != nil {
		return false
	}

	return parsedSelector.Matches(labels.Set(targetLabels))
}

// filterBackends applies filtering to backend candidates
func (s *profileSelector) filterBackends(ctx context.Context, workload *scorev1b1.Workload, backends []scorev1b1.BackendSpec) ([]scorev1b1.BackendSpec, error) {
	candidates := make([]scorev1b1.BackendSpec, 0, len(backends))

	// Get namespace labels for constraint evaluation
	namespace := &corev1.Namespace{}
	if err := s.client.Get(ctx, client.ObjectKey{Name: workload.Namespace}, namespace); err != nil {
		return nil, fmt.Errorf("failed to get namespace %q: %w", workload.Namespace, err)
	}

	combinedLabels := combineLabels(workload.Labels, namespace.Labels)

	for _, backend := range backends {
		// Apply environment selectors (skip if no constraints)
		if backend.Constraints != nil && !s.backendSelectorsMatch(backend.Constraints.Selectors, combinedLabels) {
			continue
		}

		// Validate feature requirements (skip if no constraints)
		if backend.Constraints != nil && !s.validateFeatureRequirements(workload, backend.Constraints.Features) {
			continue
		}

		// Check resource constraints (skip if no constraints)
		if backend.Constraints != nil && backend.Constraints.Resources != nil && !s.validateResourceConstraints(workload, *backend.Constraints.Resources) {
			continue
		}

		// Backend passes all filters
		candidates = append(candidates, backend)
	}

	return candidates, nil
}

// backendSelectorsMatch checks if backend constraint selectors match
func (s *profileSelector) backendSelectorsMatch(selectors []scorev1b1.SelectorSpec, targetLabels map[string]string) bool {
	// If no selectors specified, backend matches all environments
	if len(selectors) == 0 {
		return true
	}

	// At least one selector must match
	for _, selector := range selectors {
		if s.selectorMatches(selector, targetLabels) {
			return true
		}
	}

	return false
}

// validateFeatureRequirements validates features against workload requirements
func (s *profileSelector) validateFeatureRequirements(workload *scorev1b1.Workload, requiredFeatures []string) bool {
	if len(requiredFeatures) == 0 {
		return true
	}

	// Get workload feature requirements from annotation
	requirementsAnnotation, exists := workload.Annotations["score.dev/requirements"]
	if !exists {
		return len(requiredFeatures) == 0
	}

	workloadFeatures := strings.Split(requirementsAnnotation, ",")
	workloadFeatureSet := make(map[string]bool)
	for _, feature := range workloadFeatures {
		workloadFeatureSet[strings.TrimSpace(feature)] = true
	}

	// All required features must be present in workload requirements
	for _, required := range requiredFeatures {
		if !workloadFeatureSet[required] {
			return false
		}
	}

	return true
}

// validateResourceConstraints validates resource constraints (placeholder implementation)
func (s *profileSelector) validateResourceConstraints(workload *scorev1b1.Workload, constraints scorev1b1.ResourceConstraints) bool {
	// For MVP: basic validation - could be enhanced with proper quantity parsing
	// This is a simplified implementation for now
	return true
}

// selectBackend performs deterministic backend selection with sorting
func (s *profileSelector) selectBackend(candidates []scorev1b1.BackendSpec) scorev1b1.BackendSpec {
	// Sort deterministically: priority (desc) → version (SemVer desc) → backendId (asc)
	sort.Slice(candidates, func(i, j int) bool {
		a, b := candidates[i], candidates[j]

		// 1. Priority comparison (higher priority wins)
		if a.Priority != b.Priority {
			return a.Priority > b.Priority
		}

		// 2. Version comparison (SemVer, newer wins)
		versionA, errA := semver.NewVersion(a.Version)
		versionB, errB := semver.NewVersion(b.Version)

		if errA == nil && errB == nil {
			cmp := versionA.Compare(versionB)
			if cmp != 0 {
				return cmp > 0 // newer version wins
			}
		} else if errA == nil && errB != nil {
			return true // valid semver beats invalid
		} else if errA != nil && errB == nil {
			return false // invalid semver loses to valid
		}
		// If both invalid, fall through to backendId comparison

		// 3. BackendId comparison (lexicographical, smaller wins for determinism)
		return a.BackendId < b.BackendId
	})

	// Return the first (best) candidate
	return candidates[0]
}
