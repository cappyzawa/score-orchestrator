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
	"fmt"
	"net/url"
	"strconv"
	"strings"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

const (
	schemeHTTP  = "http"
	schemeHTTPS = "https"
)

// DeriveEndpoint determines the canonical endpoint for a Workload
// ADR-0003: Template-based derivation is now handled via WorkloadPlan
func DeriveEndpoint(workload *scorev1b1.Workload, template string, preferHTTPS bool) *string {
	if template == "" {
		return nil // No template available - requires runtime reporting
	}

	endpoint := renderTemplate(template, workload)

	if endpoint == "" {
		return nil
	}

	// Normalize the endpoint
	normalized := normalizeEndpoint(endpoint, &preferHTTPS)
	return &normalized
}

// renderTemplate performs basic template rendering for MVP
// Template variables supported: {{.Name}}, {{.Namespace}}
func renderTemplate(template string, workload *scorev1b1.Workload) string {
	result := template
	result = strings.ReplaceAll(result, "{{.Name}}", workload.Name)
	result = strings.ReplaceAll(result, "{{.Namespace}}", workload.Namespace)

	// Additional template variables can be added here
	// For MVP, we only support basic workload metadata

	return result
}

// normalizeEndpoint applies normalization rules to the endpoint
func normalizeEndpoint(endpoint string, preferHTTPS *bool) string {
	// Parse the URL
	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint // Return as-is if parsing fails
	}

	// Apply HTTPS preference if specified
	if preferHTTPS != nil && *preferHTTPS {
		if u.Scheme == schemeHTTP {
			u.Scheme = schemeHTTPS
		}
	}

	// Normalize port handling
	u = normalizePort(u)

	// Ensure FQDN if hostname doesn't contain dots (add cluster domain)
	if u.Host != "" && !strings.Contains(u.Hostname(), ".") {
		// For MVP, we assume cluster.local domain
		// Real implementation would get this from platform configuration
		u.Host = fmt.Sprintf("%s.%s.svc.cluster.local", u.Host, "default")
		if u.Port() != "" {
			u.Host = fmt.Sprintf("%s:%s", u.Host, u.Port())
		}
	}

	return u.String()
}

// normalizePort removes standard ports and handles non-standard ones
func normalizePort(u *url.URL) *url.URL {
	port := u.Port()
	if port == "" {
		return u
	}

	// Remove standard ports
	if (u.Scheme == schemeHTTP && port == "80") || (u.Scheme == schemeHTTPS && port == "443") {
		hostname := u.Hostname()
		u.Host = hostname
	}

	return u
}

// GetServiceBasedEndpoint derives endpoint from service configuration (future implementation)
// MVP: This is a placeholder for when service-based derivation is implemented
func GetServiceBasedEndpoint(workload *scorev1b1.Workload) *string {
	// Future implementation:
	// 1. Look for named ports (web, http, https, main)
	// 2. Handle single port case
	// 3. Apply platform-specific ingress patterns

	if workload.Spec.Service == nil || len(workload.Spec.Service.Ports) == 0 {
		return nil
	}

	// For MVP, return nil to indicate service-based derivation is not implemented
	// This will rely on WorkloadPlan templates or runtime reporting
	return nil
}

// ValidateEndpoint checks if the endpoint is a valid URI
func ValidateEndpoint(endpoint string) error {
	if endpoint == "" {
		return fmt.Errorf("endpoint cannot be empty")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if u.Scheme == "" {
		return fmt.Errorf("endpoint must include scheme (http/https)")
	}

	if u.Scheme != schemeHTTP && u.Scheme != schemeHTTPS {
		return fmt.Errorf("endpoint scheme must be http or https, got: %s", u.Scheme)
	}

	if u.Host == "" {
		return fmt.Errorf("endpoint must include host")
	}

	// Validate port if present
	if port := u.Port(); port != "" {
		if portNum, err := strconv.Atoi(port); err != nil || portNum < 1 || portNum > 65535 {
			return fmt.Errorf("invalid port number: %s", port)
		}
	}

	return nil
}

// IsHTTPS returns true if the endpoint uses HTTPS
func IsHTTPS(endpoint string) bool {
	u, err := url.Parse(endpoint)
	if err != nil {
		return false
	}
	return u.Scheme == schemeHTTPS
}

// ExtractHostPort returns the host and port from an endpoint
func ExtractHostPort(endpoint string) (string, string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", "", err
	}

	hostname := u.Hostname()
	port := u.Port()

	// Use default ports if not specified
	if port == "" {
		switch u.Scheme {
		case schemeHTTP:
			port = "80"
		case schemeHTTPS:
			port = "443"
		}
	}

	return hostname, port, nil
}
