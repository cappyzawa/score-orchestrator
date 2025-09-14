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
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
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
	profileName, err := s.selectProfile(workload)
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
	candidates := s.filterBackends(workload, selectedProfile.Backends)

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
func (s *profileSelector) selectProfile(workload *scorev1b1.Workload) (string, error) {
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

	// 3. Selector matching: Apply defaults.selectors[] based on workload labels only
	selectedProfile := s.selectProfileFromSelectors(workload)
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
// Based on ADR-0004: Only use Workload labels, not namespace labels
func (s *profileSelector) selectProfileFromSelectors(workload *scorev1b1.Workload) string {
	// Use only Workload labels (cluster-level environment model per ADR-0004)
	workloadLabels := workload.Labels
	if workloadLabels == nil {
		workloadLabels = make(map[string]string)
	}

	// Evaluate selectors in document order - first match wins
	for _, selector := range s.config.Spec.Defaults.Selectors {
		if s.selectorMatches(selector, workloadLabels) && selector.Profile != "" {
			return selector.Profile
		}
	}

	return ""
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
// Based on ADR-0004: Simplified filtering without namespace labels
func (s *profileSelector) filterBackends(workload *scorev1b1.Workload, backends []scorev1b1.BackendSpec) []scorev1b1.BackendSpec {
	candidates := make([]scorev1b1.BackendSpec, 0, len(backends))

	// Use only Workload labels (cluster-level environment model per ADR-0004)
	workloadLabels := workload.Labels
	if workloadLabels == nil {
		workloadLabels = make(map[string]string)
	}

	for _, backend := range backends {
		// Apply backend selectors using only workload labels (skip if no constraints)
		if backend.Constraints != nil && !s.backendSelectorsMatch(backend.Constraints.Selectors, workloadLabels) {
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

	return candidates
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

	// Get workload features from annotation and auto-detection
	workloadFeatureSet := s.getWorkloadFeatures(workload)

	// All required features must be present in workload capabilities
	for _, required := range requiredFeatures {
		if !workloadFeatureSet[required] {
			return false
		}
	}

	return true
}

// validateResourceConstraints validates resource constraints against workload requirements
func (s *profileSelector) validateResourceConstraints(workload *scorev1b1.Workload, constraints scorev1b1.ResourceConstraints) bool {
	// Extract total resource requirements from all containers
	totalCPU, totalMemory, totalStorage := s.calculateWorkloadResources(workload)

	// Validate CPU constraints
	if constraints.CPU != "" {
		if !s.validateQuantityConstraint(totalCPU, constraints.CPU) {
			return false
		}
	}

	// Validate Memory constraints
	if constraints.Memory != "" {
		if !s.validateQuantityConstraint(totalMemory, constraints.Memory) {
			return false
		}
	}

	// Validate Storage constraints
	if constraints.Storage != "" {
		if !s.validateQuantityConstraint(totalStorage, constraints.Storage) {
			return false
		}
	}

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

// calculateWorkloadResources calculates total resource requirements from all containers
func (s *profileSelector) calculateWorkloadResources(workload *scorev1b1.Workload) (string, string, string) {
	var totalCPU, totalMemory, totalStorage int64

	// Sum up resources from all containers
	for _, container := range workload.Spec.Containers {
		if container.Resources != nil && container.Resources.Requests != nil {
			// Parse CPU requests
			if cpuStr, exists := container.Resources.Requests["cpu"]; exists {
				if cpu, err := parseQuantity(cpuStr); err == nil {
					totalCPU += cpu
				}
			}

			// Parse Memory requests
			if memoryStr, exists := container.Resources.Requests["memory"]; exists {
				if memory, err := parseQuantity(memoryStr); err == nil {
					totalMemory += memory
				}
			}

			// Parse Storage requests (ephemeral storage)
			if storageStr, exists := container.Resources.Requests["ephemeral-storage"]; exists {
				if storage, err := parseQuantity(storageStr); err == nil {
					totalStorage += storage
				}
			}
		}
	}

	// Convert back to string format
	cpuStr := fmt.Sprintf("%dm", totalCPU)
	memoryStr := fmt.Sprintf("%d", totalMemory)
	storageStr := fmt.Sprintf("%d", totalStorage)

	return cpuStr, memoryStr, storageStr
}

// validateQuantityConstraint validates a quantity against a range constraint
func (s *profileSelector) validateQuantityConstraint(actual, constraint string) bool {
	if constraint == "" {
		return true
	}

	// Parse constraint format: "100m-4000m", "100m-", "-4000m", or "100m"
	parts := strings.Split(constraint, "-")

	switch len(parts) {
	case 1:
		// Exact match: "100m"
		return actual == constraint
	case 2:
		minStr, maxStr := parts[0], parts[1]

		// Parse actual value
		actualQty, err := parseQuantity(actual)
		if err != nil {
			return false
		}

		// Check minimum (if specified)
		if minStr != "" {
			minQty, err := parseQuantity(minStr)
			if err != nil {
				return false
			}
			if actualQty < minQty {
				return false
			}
		}

		// Check maximum (if specified)
		if maxStr != "" {
			maxQty, err := parseQuantity(maxStr)
			if err != nil {
				return false
			}
			if actualQty > maxQty {
				return false
			}
		}

		return true
	default:
		return false
	}
}

// parseQuantity parses a Kubernetes quantity string to a comparable value
func parseQuantity(quantityStr string) (int64, error) {
	if quantityStr == "" {
		return 0, nil
	}

	// Simple parsing for common units (millicpu, memory)
	if strings.HasSuffix(quantityStr, "m") {
		// CPU millicores
		value := strings.TrimSuffix(quantityStr, "m")
		return strconv.ParseInt(value, 10, 64)
	}

	if strings.HasSuffix(quantityStr, "Mi") {
		// Memory in MiB
		value := strings.TrimSuffix(quantityStr, "Mi")
		val, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, err
		}
		return val * 1024 * 1024, nil
	}

	if strings.HasSuffix(quantityStr, "Gi") {
		// Memory in GiB
		value := strings.TrimSuffix(quantityStr, "Gi")
		val, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, err
		}
		return val * 1024 * 1024 * 1024, nil
	}

	// Plain number (bytes)
	return strconv.ParseInt(quantityStr, 10, 64)
}

// getWorkloadFeatures returns a set of workload features (from annotation and auto-detection)
func (s *profileSelector) getWorkloadFeatures(workload *scorev1b1.Workload) map[string]bool {
	featureSet := make(map[string]bool)

	// Get explicit requirements from annotation
	if requirementsAnnotation, exists := workload.Annotations["score.dev/requirements"]; exists {
		workloadFeatures := strings.Split(requirementsAnnotation, ",")
		for _, feature := range workloadFeatures {
			featureSet[strings.TrimSpace(feature)] = true
		}
	}

	// Auto-detect features from workload characteristics
	s.autoDetectFeatures(workload, featureSet)

	return featureSet
}

// autoDetectFeatures automatically detects features based on workload characteristics
func (s *profileSelector) autoDetectFeatures(workload *scorev1b1.Workload, featureSet map[string]bool) {
	s.detectIngressFeatures(workload, featureSet)
	s.detectMonitoringFeatures(workload, featureSet)
	s.detectScaleFeatures(workload, featureSet)
	s.detectStorageFeatures(workload, featureSet)
	s.detectDatabaseFeatures(workload, featureSet)
}

// detectIngressFeatures detects HTTP/HTTPS ingress capabilities
func (s *profileSelector) detectIngressFeatures(workload *scorev1b1.Workload, featureSet map[string]bool) {
	if workload.Spec.Service == nil || len(workload.Spec.Service.Ports) == 0 {
		return
	}

	for _, port := range workload.Spec.Service.Ports {
		// Check for common HTTP ports
		if port.Port == 80 || port.Port == 8080 || port.Port == 3000 || port.Port == 8000 {
			featureSet["http-ingress"] = true
		}
		// Check for HTTPS ports
		if port.Port == 443 || port.Port == 8443 {
			featureSet["https-ingress"] = true
			featureSet["http-ingress"] = true // HTTPS implies HTTP capability
		}
	}

	// If any service port is exposed, it likely needs ingress capability
	featureSet["http-ingress"] = true
}

// detectMonitoringFeatures detects monitoring and metrics capabilities
func (s *profileSelector) detectMonitoringFeatures(workload *scorev1b1.Workload, featureSet map[string]bool) {
	// Check service ports for monitoring
	if workload.Spec.Service != nil {
		for _, port := range workload.Spec.Service.Ports {
			// Common monitoring/metrics ports
			if port.Port == 9090 || port.Port == 9100 || port.Port == 3000 {
				featureSet["monitoring"] = true
				return
			}
		}
	}

	// Check container environment variables
	for _, container := range workload.Spec.Containers {
		if container.Variables == nil {
			continue
		}
		for varName := range container.Variables {
			upperVarName := strings.ToUpper(varName)
			if strings.Contains(upperVarName, "METRICS") ||
				strings.Contains(upperVarName, "MONITORING") ||
				strings.Contains(upperVarName, "PROMETHEUS") {
				featureSet["monitoring"] = true
				return
			}
		}
	}
}

// detectScaleFeatures detects scale-to-zero capabilities
func (s *profileSelector) detectScaleFeatures(workload *scorev1b1.Workload, featureSet map[string]bool) {
	if scaleAnnotation, exists := workload.Annotations["score.dev/scale-to-zero"]; exists && scaleAnnotation == "true" {
		featureSet["scale-to-zero"] = true
	}
}

// detectStorageFeatures detects persistent storage requirements
func (s *profileSelector) detectStorageFeatures(workload *scorev1b1.Workload, featureSet map[string]bool) {
	for _, container := range workload.Spec.Containers {
		if len(container.Files) > 0 {
			featureSet["persistent-storage"] = true
			return
		}
	}
}

// detectDatabaseFeatures detects database connectivity requirements
func (s *profileSelector) detectDatabaseFeatures(workload *scorev1b1.Workload, featureSet map[string]bool) {
	if workload.Spec.Resources == nil {
		return
	}

	for _, resource := range workload.Spec.Resources {
		resourceType := strings.ToLower(resource.Type)
		if strings.Contains(resourceType, "postgres") ||
			strings.Contains(resourceType, "mysql") ||
			strings.Contains(resourceType, "database") ||
			strings.Contains(resourceType, "redis") {
			featureSet["database-connectivity"] = true
			return
		}
	}
}
