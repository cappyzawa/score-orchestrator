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

package config

import (
	"regexp"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"

	scorev1b1 "github.com/cappyzawa/score-orchestrator/api/v1b1"
)

// Validator validates orchestrator configuration
type Validator struct {
	// validTemplateKinds defines the supported template kinds
	validTemplateKinds map[string]bool

	// validRuntimeClasses defines the supported runtime classes
	validRuntimeClasses map[string]bool

	// dnsLabelRegex validates DNS label format
	dnsLabelRegex *regexp.Regexp
}

// NewValidator creates a new configuration validator
func NewValidator() *Validator {
	return &Validator{
		validTemplateKinds: map[string]bool{
			"manifests": true,
			"helm":      true,
			"kustomize": true,
		},
		validRuntimeClasses: map[string]bool{
			"kubernetes": true,
			"ecs":        true,
			"nomad":      true,
		},
		dnsLabelRegex: regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`),
	}
}

// Validate validates the orchestrator configuration
func (v *Validator) Validate(config *scorev1b1.OrchestratorConfig) error {
	var allErrs field.ErrorList

	// Validate metadata
	if config.APIVersion != "score.dev/v1b1" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("apiVersion"), config.APIVersion, "must be 'score.dev/v1b1'"))
	}

	if config.Kind != "OrchestratorConfig" {
		allErrs = append(allErrs, field.Invalid(field.NewPath("kind"), config.Kind, "must be 'OrchestratorConfig'"))
	}

	if config.Metadata.Name == "" {
		allErrs = append(allErrs, field.Required(field.NewPath("metadata", "name"), "name is required"))
	}

	// Validate spec
	specPath := field.NewPath("spec")
	allErrs = append(allErrs, v.validateProfiles(config.Spec.Profiles, specPath.Child("profiles"))...)
	allErrs = append(allErrs, v.validateProvisioners(config.Spec.Provisioners, specPath.Child("provisioners"))...)
	allErrs = append(allErrs, v.validateDefaults(&config.Spec.Defaults, specPath.Child("defaults"))...)

	// Validate cross-references
	allErrs = append(allErrs, v.validateCrossReferences(config)...)

	if len(allErrs) > 0 {
		return allErrs.ToAggregate()
	}

	return nil
}

// validateProfiles validates the profiles section
func (v *Validator) validateProfiles(profiles []scorev1b1.ProfileSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	if len(profiles) == 0 {
		allErrs = append(allErrs, field.Required(fldPath, "at least one profile must be defined"))
		return allErrs
	}

	profileNames := make(map[string]bool)
	backendIds := make(map[string]bool)

	for i, profile := range profiles {
		profilePath := fldPath.Index(i)

		// Validate profile name
		if profile.Name == "" {
			allErrs = append(allErrs, field.Required(profilePath.Child("name"), "name is required"))
		} else {
			if errs := validation.IsDNS1123Label(profile.Name); len(errs) > 0 {
				allErrs = append(allErrs, field.Invalid(profilePath.Child("name"), profile.Name, strings.Join(errs, "; ")))
			}
			if profileNames[profile.Name] {
				allErrs = append(allErrs, field.Duplicate(profilePath.Child("name"), profile.Name))
			}
			profileNames[profile.Name] = true
		}

		// Validate backends
		if len(profile.Backends) == 0 {
			allErrs = append(allErrs, field.Required(profilePath.Child("backends"), "at least one backend must be defined"))
		}

		for j, backend := range profile.Backends {
			backendPath := profilePath.Child("backends").Index(j)
			allErrs = append(allErrs, v.validateBackend(&backend, backendPath, backendIds)...)
		}
	}

	return allErrs
}

// validateBackend validates a single backend
func (v *Validator) validateBackend(backend *scorev1b1.BackendSpec, fldPath *field.Path, backendIds map[string]bool) field.ErrorList {
	var allErrs field.ErrorList

	// Validate backend ID
	if backend.BackendId == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("backendId"), "backendId is required"))
	} else {
		if backendIds[backend.BackendId] {
			allErrs = append(allErrs, field.Duplicate(fldPath.Child("backendId"), backend.BackendId))
		}
		backendIds[backend.BackendId] = true
	}

	// Validate runtime class
	if backend.RuntimeClass == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("runtimeClass"), "runtimeClass is required"))
	} else if !v.validRuntimeClasses[backend.RuntimeClass] {
		validClasses := make([]string, 0, len(v.validRuntimeClasses))
		for class := range v.validRuntimeClasses {
			validClasses = append(validClasses, class)
		}
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("runtimeClass"), backend.RuntimeClass, validClasses))
	}

	// Validate template
	allErrs = append(allErrs, v.validateTemplate(&backend.Template, fldPath.Child("template"))...)

	// Validate version (should be semver but we'll do basic validation)
	if backend.Version == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("version"), "version is required"))
	}

	// Validate priority (must be non-negative)
	if backend.Priority < 0 {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("priority"), backend.Priority, "priority must be non-negative"))
	}

	// Validate constraints if present
	if backend.Constraints != nil {
		allErrs = append(allErrs, v.validateConstraints(backend.Constraints, fldPath.Child("constraints"))...)
	}

	return allErrs
}

// validateTemplate validates a template specification
func (v *Validator) validateTemplate(template *scorev1b1.TemplateSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// Validate kind
	if template.Kind == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("kind"), "kind is required"))
	} else if !v.validTemplateKinds[template.Kind] {
		validKinds := make([]string, 0, len(v.validTemplateKinds))
		for kind := range v.validTemplateKinds {
			validKinds = append(validKinds, kind)
		}
		allErrs = append(allErrs, field.NotSupported(fldPath.Child("kind"), template.Kind, validKinds))
	}

	// Validate ref
	if template.Ref == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("ref"), "ref is required"))
	}

	return allErrs
}

// validateConstraints validates constraint specifications
func (v *Validator) validateConstraints(constraints *scorev1b1.ConstraintsSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// Validate resource constraints if present
	if constraints.Resources != nil {
		allErrs = append(allErrs, v.validateResourceConstraints(constraints.Resources, fldPath.Child("resources"))...)
	}

	return allErrs
}

// validateResourceConstraints validates resource constraint specifications
func (v *Validator) validateResourceConstraints(resources *scorev1b1.ResourceConstraints, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// Basic validation for resource constraint format
	// Format should be "<quantity>" or "<min>-<max>"
	constraintPattern := regexp.MustCompile(`^([0-9]+[a-zA-Z]*|[0-9]+[a-zA-Z]*-[0-9]+[a-zA-Z]*|-[0-9]+[a-zA-Z]*|[0-9]+[a-zA-Z]*-)$`)

	if resources.CPU != "" && !constraintPattern.MatchString(resources.CPU) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("cpu"), resources.CPU, "invalid CPU constraint format"))
	}

	if resources.Memory != "" && !constraintPattern.MatchString(resources.Memory) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("memory"), resources.Memory, "invalid memory constraint format"))
	}

	if resources.Storage != "" && !constraintPattern.MatchString(resources.Storage) {
		allErrs = append(allErrs, field.Invalid(fldPath.Child("storage"), resources.Storage, "invalid storage constraint format"))
	}

	return allErrs
}

// validateProvisioners validates the provisioners section
func (v *Validator) validateProvisioners(provisioners []scorev1b1.ProvisionerSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	provisionerTypes := make(map[string]bool)

	for i, provisioner := range provisioners {
		provisionerPath := fldPath.Index(i)

		// Validate type
		if provisioner.Type == "" {
			allErrs = append(allErrs, field.Required(provisionerPath.Child("type"), "type is required"))
		} else {
			if provisionerTypes[provisioner.Type] {
				allErrs = append(allErrs, field.Duplicate(provisionerPath.Child("type"), provisioner.Type))
			}
			provisionerTypes[provisioner.Type] = true
		}

		// Validate provisioner
		if provisioner.Provisioner == "" {
			allErrs = append(allErrs, field.Required(provisionerPath.Child("provisioner"), "provisioner is required"))
		}

		// Validate classes
		allErrs = append(allErrs, v.validateClasses(provisioner.Classes, provisionerPath.Child("classes"))...)
	}

	return allErrs
}

// validateClasses validates provisioner classes
func (v *Validator) validateClasses(classes []scorev1b1.ClassSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	classNames := make(map[string]bool)

	for i, class := range classes {
		classPath := fldPath.Index(i)

		// Validate name
		if class.Name == "" {
			allErrs = append(allErrs, field.Required(classPath.Child("name"), "name is required"))
		} else {
			if classNames[class.Name] {
				allErrs = append(allErrs, field.Duplicate(classPath.Child("name"), class.Name))
			}
			classNames[class.Name] = true
		}

		// Validate constraints if present
		if class.Constraints != nil {
			allErrs = append(allErrs, v.validateConstraints(class.Constraints, classPath.Child("constraints"))...)
		}
	}

	return allErrs
}

// validateDefaults validates the defaults section
func (v *Validator) validateDefaults(defaults *scorev1b1.DefaultsSpec, fldPath *field.Path) field.ErrorList {
	var allErrs field.ErrorList

	// Validate default profile
	if defaults.Profile == "" {
		allErrs = append(allErrs, field.Required(fldPath.Child("profile"), "default profile is required"))
	}

	// Validate selectors
	for i, selector := range defaults.Selectors {
		selectorPath := fldPath.Child("selectors").Index(i)

		// At least one of matchLabels or matchExpressions must be specified
		if len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0 {
			allErrs = append(allErrs, field.Required(selectorPath, "selector must specify either matchLabels or matchExpressions"))
		}

		// Validate constraints if present
		if selector.Constraints != nil {
			allErrs = append(allErrs, v.validateConstraints(selector.Constraints, selectorPath.Child("constraints"))...)
		}
	}

	return allErrs
}

// validateCrossReferences validates cross-references between different parts of the configuration
func (v *Validator) validateCrossReferences(config *scorev1b1.OrchestratorConfig) field.ErrorList {
	var allErrs field.ErrorList

	// Collect all profile names
	profileNames := make(map[string]bool)
	for _, profile := range config.Spec.Profiles {
		profileNames[profile.Name] = true
	}

	// Validate default profile reference
	if config.Spec.Defaults.Profile != "" && !profileNames[config.Spec.Defaults.Profile] {
		allErrs = append(allErrs, field.NotFound(field.NewPath("spec", "defaults", "profile"), config.Spec.Defaults.Profile))
	}

	// Validate selector profile references
	for i, selector := range config.Spec.Defaults.Selectors {
		if selector.Profile != "" && !profileNames[selector.Profile] {
			allErrs = append(allErrs, field.NotFound(field.NewPath("spec", "defaults", "selectors").Index(i).Child("profile"), selector.Profile))
		}
	}

	// Validate provisioner class references
	for i, provisioner := range config.Spec.Provisioners {
		if provisioner.Defaults != nil && provisioner.Defaults.Class != "" {
			// Check if the referenced class exists
			classExists := false
			for _, class := range provisioner.Classes {
				if class.Name == provisioner.Defaults.Class {
					classExists = true
					break
				}
			}
			if !classExists {
				allErrs = append(allErrs, field.NotFound(field.NewPath("spec", "provisioners").Index(i).Child("defaults", "class"), provisioner.Defaults.Class))
			}
		}
	}

	return allErrs
}
