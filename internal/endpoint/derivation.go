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

package endpoint

import (
	"context"
	"fmt"
	"sort"
	"strings"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// EndpointDeriver derives endpoints from WorkloadPlan templates and service configurations
type EndpointDeriver struct {
	client client.Client
}

// NewEndpointDeriver creates a new endpoint deriver
func NewEndpointDeriver(c client.Client) *EndpointDeriver {
	return &EndpointDeriver{
		client: c,
	}
}

// DeriveEndpoint derives the canonical endpoint for a Workload from its WorkloadPlan
func (e *EndpointDeriver) DeriveEndpoint(
	ctx context.Context,
	workload *scorev1b1.Workload,
	plan *scorev1b1.WorkloadPlan,
) (string, error) {
	if plan == nil {
		return "", nil
	}

	// 1. Check template endpoint configuration
	if plan.Spec.Template != nil && plan.Spec.Values != nil {
		if templateEndpoint, err := e.extractTemplateEndpoint(plan); err == nil && templateEndpoint != "" {
			return e.normalizeEndpoint(templateEndpoint, workload, plan)
		}
	}

	// 2. Apply service port name prioritization
	if serviceEndpoint := e.deriveFromServicePorts(workload); serviceEndpoint != "" {
		return e.normalizeEndpoint(serviceEndpoint, workload, plan)
	}

	// 3. No endpoint derivable
	return "", nil
}

// extractTemplateEndpoint extracts endpoint from template values
func (e *EndpointDeriver) extractTemplateEndpoint(plan *scorev1b1.WorkloadPlan) (string, error) {
	// For MVP, we assume template values contain an "endpoint" field
	// In a real implementation, this would parse the template values
	// and extract endpoint configuration based on template kind

	// TODO: Implement actual template parsing based on template.Kind
	// For now, return empty to fall back to service-based derivation
	return "", nil
}

// deriveFromServicePorts derives endpoint from service port configuration
func (e *EndpointDeriver) deriveFromServicePorts(workload *scorev1b1.Workload) string {
	service := workload.Spec.Service
	if service == nil || len(service.Ports) == 0 {
		return ""
	}

	// Get prioritized port
	port := e.selectBestPort(service.Ports)
	if port == nil {
		return ""
	}

	// Generate base endpoint
	scheme := e.getSchemeForPort(port)
	hostname := e.generateHostname(workload)

	// Build endpoint
	endpoint := fmt.Sprintf("%s://%s", scheme, hostname)

	// Add port if non-standard
	if !e.isStandardPort(scheme, port.Port) {
		endpoint = fmt.Sprintf("%s:%d", endpoint, port.Port)
	}

	return endpoint
}

// selectBestPort selects the best port using prioritization rules
func (e *EndpointDeriver) selectBestPort(ports []scorev1b1.ServicePort) *scorev1b1.ServicePort {
	if len(ports) == 0 {
		return nil
	}

	if len(ports) == 1 {
		return &ports[0]
	}

	// Prioritize by port characteristics (since Score spec doesn't have named ports)
	prioritized := e.prioritizePortsByCharacteristics(ports)
	if len(prioritized) > 0 {
		return &prioritized[0]
	}

	// Fallback to first port
	return &ports[0]
}

// prioritizePortsByCharacteristics prioritizes ports by port number characteristics
// Priority: HTTPS ports (443, 8443) > HTTP ports (80, 8080) > others
func (e *EndpointDeriver) prioritizePortsByCharacteristics(ports []scorev1b1.ServicePort) []scorev1b1.ServicePort {
	// Create a slice of ports with their priority scores
	type portWithPriority struct {
		port     scorev1b1.ServicePort
		priority int
		isHTTPS  bool
	}

	prioritizedPorts := make([]portWithPriority, 0, len(ports))

	for _, port := range ports {
		p := portWithPriority{
			port:    port,
			isHTTPS: e.isHTTPSPort(port.Port),
		}

		// Assign priority based on port characteristics
		switch {
		case e.isHTTPSPort(port.Port):
			p.priority = 1 // HTTPS ports have highest priority
		case e.isHTTPPort(port.Port):
			p.priority = 2 // HTTP ports have second priority
		default:
			p.priority = 10 // Other ports have lower priority
		}

		prioritizedPorts = append(prioritizedPorts, p)
	}

	// Sort by priority (lower number = higher priority), then prefer HTTPS
	sort.Slice(prioritizedPorts, func(i, j int) bool {
		if prioritizedPorts[i].priority != prioritizedPorts[j].priority {
			return prioritizedPorts[i].priority < prioritizedPorts[j].priority
		}
		// If same priority, prefer HTTPS
		return prioritizedPorts[i].isHTTPS && !prioritizedPorts[j].isHTTPS
	})

	// Extract ports from sorted slice
	result := make([]scorev1b1.ServicePort, len(prioritizedPorts))
	for i, p := range prioritizedPorts {
		result[i] = p.port
	}

	return result
}

// isHTTPSPort determines if a port should use HTTPS scheme based on port number
func (e *EndpointDeriver) isHTTPSPort(port int32) bool {
	return port == 443 || port == 8443
}

// isHTTPPort determines if a port is a common HTTP port
func (e *EndpointDeriver) isHTTPPort(port int32) bool {
	return port == 80 || port == 8080
}

// getSchemeForPort returns the appropriate scheme for a port
func (e *EndpointDeriver) getSchemeForPort(port *scorev1b1.ServicePort) string {
	if e.isHTTPSPort(port.Port) {
		return schemeHTTPS
	}
	return schemeHTTP
}

// generateHostname generates hostname for the workload
func (e *EndpointDeriver) generateHostname(workload *scorev1b1.Workload) string {
	// For MVP, generate a simple hostname
	// In production, this would be based on ingress configuration
	return fmt.Sprintf("%s.%s.svc.cluster.local", workload.Name, workload.Namespace)
}

// isStandardPort checks if the port is standard for the scheme
func (e *EndpointDeriver) isStandardPort(scheme string, port int32) bool {
	return (scheme == schemeHTTP && port == 80) ||
		(scheme == schemeHTTPS && port == 443)
}

// normalizeEndpoint applies normalization rules to the derived endpoint
func (e *EndpointDeriver) normalizeEndpoint(endpoint string, workload *scorev1b1.Workload, _ *scorev1b1.WorkloadPlan) (string, error) {
	if endpoint == "" {
		return "", nil
	}

	// Apply template rendering if needed
	rendered := e.renderEndpointTemplate(endpoint, workload)

	// Apply existing normalization from derive.go without HTTPS preference
	// The scheme should be determined by port selection logic, not forced
	normalized := normalizeEndpoint(rendered, nil)

	// Validate the final endpoint
	if err := ValidateEndpoint(normalized); err != nil {
		return "", fmt.Errorf("invalid derived endpoint: %w", err)
	}

	return normalized, nil
}

// renderEndpointTemplate renders template variables in endpoint
func (e *EndpointDeriver) renderEndpointTemplate(endpoint string, workload *scorev1b1.Workload) string {
	result := endpoint
	result = strings.ReplaceAll(result, "{{.Name}}", workload.Name)
	result = strings.ReplaceAll(result, "{{.Namespace}}", workload.Namespace)

	// Add more template variables as needed
	return result
}
