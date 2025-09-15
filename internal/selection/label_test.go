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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = DescribeTable("CombineLabels",
	func(workloadLabels, namespaceLabels, expected map[string]string) {
		result := combineLabels(workloadLabels, namespaceLabels)
		Expect(result).To(Equal(expected))
	},
	Entry("workload labels take precedence over namespace labels",
		map[string]string{
			"app":         "myapp",
			"environment": "staging", // conflicts with namespace
		},
		map[string]string{
			"environment": "production", // should be overridden
			"team":        "backend",
		},
		map[string]string{
			"app":         "myapp",
			"environment": "staging", // workload wins
			"team":        "backend",
		},
	),
	Entry("only namespace labels",
		nil,
		map[string]string{
			"environment": "production",
			"team":        "backend",
		},
		map[string]string{
			"environment": "production",
			"team":        "backend",
		},
	),
	Entry("only workload labels",
		map[string]string{
			"app":     "myapp",
			"version": "1.0.0",
		},
		nil,
		map[string]string{
			"app":     "myapp",
			"version": "1.0.0",
		},
	),
	Entry("both nil",
		nil,
		nil,
		map[string]string{},
	),
	Entry("empty maps",
		map[string]string{},
		map[string]string{},
		map[string]string{},
	),
	Entry("no conflicts",
		map[string]string{
			"app":     "myapp",
			"version": "1.0.0",
		},
		map[string]string{
			"environment": "production",
			"team":        "backend",
		},
		map[string]string{
			"app":         "myapp",
			"version":     "1.0.0",
			"environment": "production",
			"team":        "backend",
		},
	),
)
