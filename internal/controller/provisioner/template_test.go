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
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("TemplateEngine", func() {
	var (
		engine *TemplateEngine
		ctx    *TemplateContext
	)

	BeforeEach(func() {
		engine = NewTemplateEngine()
		ctx = &TemplateContext{
			ClaimName: "test-claim",
			ClaimKey:  "database",
			Namespace: "test-namespace",
			Type:      "postgres",
			ClassParams: map[string]interface{}{
				"cpu":     "500m",
				"memory":  "1Gi",
				"storage": "10Gi",
			},
			Secrets: map[string]string{
				"password": "secret123",
				"username": "postgres",
			},
			Services: map[string]string{
				"name": "test-claim-postgres",
				"port": "5432",
			},
		}
	})

	Context("When substituting template variables", func() {
		It("should substitute basic variables", func() {
			template := "Name: {{.ClaimName}}, Namespace: {{.Namespace}}, Type: {{.Type}}"

			result, err := engine.Substitute(template, ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Name: test-claim, Namespace: test-namespace, Type: postgres"))
		})

		It("should substitute nested variables", func() {
			template := "CPU: {{.ClassParams.cpu}}, Memory: {{.ClassParams.memory}}"

			result, err := engine.Substitute(template, ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("CPU: 500m, Memory: 1Gi"))
		})

		It("should substitute secret variables", func() {
			template := "postgresql://{{.Secrets.username}}:{{.Secrets.password}}@{{.Services.name}}:{{.Services.port}}/postgres"

			result, err := engine.Substitute(template, ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("postgresql://postgres:secret123@test-claim-postgres:5432/postgres"))
		})

		It("should handle missing variables gracefully", func() {
			template := "Value: {{.NonExistentField}}"

			result, err := engine.Substitute(template, ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Value: <no value>"))
		})

		It("should handle template functions", func() {
			template := "Lower: {{lower .Type}}, Upper: {{upper .Type}}"

			result, err := engine.Substitute(template, ctx)

			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Lower: postgres, Upper: POSTGRES"))
		})

		It("should return error for invalid templates", func() {
			template := "Invalid: {{.ClaimName"

			_, err := engine.Substitute(template, ctx)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to parse template"))
		})
	})

	Context("When substituting JSON data", func() {
		It("should substitute variables in JSON objects", func() {
			input := map[string]interface{}{
				"name":      "{{.ClaimName}}-service",
				"namespace": "{{.Namespace}}",
				"spec": map[string]interface{}{
					"replicas": 1,
					"image":    "postgres:13",
					"env": map[string]interface{}{
						"POSTGRES_PASSWORD": "{{.Secrets.password}}",
						"POSTGRES_USER":     "{{.Secrets.username}}",
					},
				},
			}

			result, err := engine.SubstituteJSON(input, ctx)

			Expect(err).NotTo(HaveOccurred())

			resultMap := result.(map[string]interface{})
			Expect(resultMap["name"]).To(Equal("test-claim-service"))
			Expect(resultMap["namespace"]).To(Equal("test-namespace"))

			spec := resultMap["spec"].(map[string]interface{})
			env := spec["env"].(map[string]interface{})
			Expect(env["POSTGRES_PASSWORD"]).To(Equal("secret123"))
			Expect(env["POSTGRES_USER"]).To(Equal("postgres"))
		})

		It("should handle complex nested JSON structures", func() {
			input := []interface{}{
				map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]interface{}{
						"name":      "{{.ClaimName}}-postgres",
						"namespace": "{{.Namespace}}",
					},
					"spec": map[string]interface{}{
						"template": map[string]interface{}{
							"spec": map[string]interface{}{
								"containers": []interface{}{
									map[string]interface{}{
										"name":  "postgres",
										"image": "postgres:13",
										"env": []interface{}{
											map[string]interface{}{
												"name":  "POSTGRES_PASSWORD",
												"value": "{{.Secrets.password}}",
											},
										},
									},
								},
							},
						},
					},
				},
			}

			result, err := engine.SubstituteJSON(input, ctx)

			Expect(err).NotTo(HaveOccurred())

			resultArray := result.([]interface{})
			Expect(resultArray).To(HaveLen(1))

			deployment := resultArray[0].(map[string]interface{})
			metadata := deployment["metadata"].(map[string]interface{})
			Expect(metadata["name"]).To(Equal("test-claim-postgres"))
			Expect(metadata["namespace"]).To(Equal("test-namespace"))
		})
	})

	Context("When converting TemplateContext to TemplateData", func() {
		It("should convert all fields correctly", func() {
			// Add some params as RawExtension
			params := map[string]interface{}{
				"customParam": "customValue",
				"replicas":    3,
			}
			paramsBytes, _ := json.Marshal(params)
			ctx.Params = &runtime.RawExtension{Raw: paramsBytes}

			data := ctx.ToTemplateData()

			Expect(data.ClaimName).To(Equal("test-claim"))
			Expect(data.ClaimKey).To(Equal("database"))
			Expect(data.Namespace).To(Equal("test-namespace"))
			Expect(data.Type).To(Equal("postgres"))
			Expect(data.Class).To(Equal(ctx.ClassParams))
			Expect(data.Secret).To(Equal(ctx.Secrets))
			Expect(data.Service).To(Equal(ctx.Services))
			Expect(data.Params).To(Equal(params))
		})

		It("should handle nil Params gracefully", func() {
			ctx.Params = nil

			data := ctx.ToTemplateData()

			Expect(data.Params).To(BeNil())
			Expect(data.ClaimName).To(Equal("test-claim"))
		})
	})

	Context("When populating template context", func() {
		It("should populate context with class parameters", func() {
			templateCtx := &TemplateContext{
				ClaimName: "test-claim",
				Namespace: "test-namespace",
				Type:      "postgres",
			}

			classParams := map[string]interface{}{
				"cpu":    "1000m",
				"memory": "2Gi",
			}

			PopulateTemplateContext(templateCtx, classParams)

			Expect(templateCtx.ClassParams).To(Equal(classParams))
			Expect(templateCtx.Secrets).NotTo(BeNil())
			Expect(templateCtx.Services).NotTo(BeNil())
			Expect(templateCtx.Secrets["password"]).NotTo(BeEmpty())
			Expect(templateCtx.Services["name"]).To(Equal("test-claim-postgres"))
		})

		It("should preserve existing secrets and services", func() {
			templateCtx := &TemplateContext{
				ClaimName: "test-claim",
				Type:      "postgres",
				Secrets: map[string]string{
					"existingSecret": "existingValue",
				},
				Services: map[string]string{
					"existingService": "existingValue",
				},
			}

			PopulateTemplateContext(templateCtx, make(map[string]interface{}))

			Expect(templateCtx.Secrets["existingSecret"]).To(Equal("existingValue"))
			Expect(templateCtx.Services["existingService"]).To(Equal("existingValue"))
			Expect(templateCtx.Secrets["password"]).NotTo(BeEmpty())              // Should be generated
			Expect(templateCtx.Services["name"]).To(Equal("test-claim-postgres")) // Should be set
		})
	})
})
