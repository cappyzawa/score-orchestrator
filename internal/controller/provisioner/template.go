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
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"crypto/rand"
	"encoding/base64"
)

// TemplateEngine handles template variable substitution
type TemplateEngine struct{}

// NewTemplateEngine creates a new template engine
func NewTemplateEngine() *TemplateEngine {
	return &TemplateEngine{}
}

// Substitute performs template variable substitution on the input string
func (e *TemplateEngine) Substitute(input string, ctx *TemplateContext) (string, error) {
	// Create template with helper functions
	tmpl, err := template.New("provisioner").Funcs(e.templateFunctions()).Parse(input)
	if err != nil {
		return "", fmt.Errorf("failed to parse template: %w", err)
	}

	// Execute template with context
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// SubstituteJSON performs template substitution on JSON data
func (e *TemplateEngine) SubstituteJSON(input interface{}, ctx *TemplateContext) (interface{}, error) {
	// Convert to JSON string first
	jsonBytes, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal to JSON: %w", err)
	}

	// Perform string substitution
	substituted, err := e.Substitute(string(jsonBytes), ctx)
	if err != nil {
		return nil, err
	}

	// Parse back to interface{}
	var result interface{}
	if err := json.Unmarshal([]byte(substituted), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal substituted JSON: %w", err)
	}

	return result, nil
}

// templateFunctions returns template helper functions
func (e *TemplateEngine) templateFunctions() template.FuncMap {
	return template.FuncMap{
		"generateSecret": e.generateSecret,
		"lower":          strings.ToLower,
		"upper":          strings.ToUpper,
		"replace":        strings.ReplaceAll,
	}
}

// generateSecret generates a random secret value
func (e *TemplateEngine) generateSecret(length int) (string, error) {
	if length <= 0 {
		length = 32 // default length
	}

	randomBytes := make([]byte, length)
	if _, err := rand.Read(randomBytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	return base64.URLEncoding.EncodeToString(randomBytes)[:length], nil
}

// PopulateTemplateContext enriches the template context with additional data
func PopulateTemplateContext(ctx *TemplateContext, classParams map[string]interface{}) {
	// Set class parameters
	ctx.ClassParams = classParams

	// Initialize secrets map if not already set
	if ctx.Secrets == nil {
		ctx.Secrets = make(map[string]string)
	}

	// Initialize services map if not already set
	if ctx.Services == nil {
		ctx.Services = make(map[string]string)
	}

	// Generate default secrets if needed
	if _, exists := ctx.Secrets["password"]; !exists {
		// Generate a random password
		password, err := generateRandomPassword(16)
		if err == nil {
			ctx.Secrets["password"] = password
		}
	}

	// Set default service names
	if _, exists := ctx.Services["name"]; !exists {
		ctx.Services["name"] = fmt.Sprintf("%s-%s", ctx.ClaimName, ctx.Type)
	}
}

// generateRandomPassword generates a random password of specified length
func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return string(b), nil
}

// TemplateData represents the data structure available in templates
type TemplateData struct {
	ClaimName string                 `json:"claimName"`
	ClaimKey  string                 `json:"claimKey"`
	Namespace string                 `json:"namespace"`
	Type      string                 `json:"type"`
	Class     map[string]interface{} `json:"class"`
	Params    map[string]interface{} `json:"params"`
	Secret    map[string]string      `json:"secret"`
	Service   map[string]string      `json:"service"`
	Response  map[string]interface{} `json:"response,omitempty"`
}

// ToTemplateData converts TemplateContext to TemplateData for use in templates
func (ctx *TemplateContext) ToTemplateData() *TemplateData {
	data := &TemplateData{
		ClaimName: ctx.ClaimName,
		ClaimKey:  ctx.ClaimKey,
		Namespace: ctx.Namespace,
		Type:      ctx.Type,
		Class:     ctx.ClassParams,
		Secret:    ctx.Secrets,
		Service:   ctx.Services,
		Response:  ctx.Response,
	}

	// Convert Params if available
	if ctx.Params != nil {
		var params map[string]interface{}
		if err := json.Unmarshal(ctx.Params.Raw, &params); err == nil {
			data.Params = params
		}
	}

	return data
}
