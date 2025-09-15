//go:build e2e
// +build e2e

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

package e2e

import (
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cappyzawa/score-orchestrator/test/utils"
)

var _ = Describe("ADR-0003/0004 Workload Lifecycle E2E Tests", func() {

	BeforeEach(func() {
		By("Installing CRDs for Workload E2E tests")
		cmd := exec.Command("make", "install")
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred(), "Failed to install CRDs")
	})

	AfterEach(func() {
		By("Cleaning up test resources")
		// Remove finalizer first to allow clean deletion
		cmd := exec.Command("kubectl", "patch", "workload", "basic-web-test", "-n", "kbinit-system", "--type=merge", "-p", `{"metadata":{"finalizers":[]}}`, "--ignore-not-found=true")
		utils.Run(cmd)
		// Then delete the workload
		cmd = exec.Command("kubectl", "delete", "workload", "basic-web-test", "-n", "kbinit-system", "--ignore-not-found=true", "--timeout=30s")
		utils.Run(cmd)
	})

	Context("Complete Pipeline - Basic Web Service", func() {
		It("should validate workload inputs successfully", func() {
			By("Applying basic web service workload")
			cmd := exec.Command("kubectl", "apply", "-f", "test/e2e/fixtures/workloads/basic-web.yaml")
			_, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred(), "Failed to apply workload")

			By("Checking InputsValid condition becomes True")
			Eventually(func() bool {
				cmd := exec.Command("kubectl", "get", "workload", "basic-web-test", "-n", "kbinit-system", "-o", "jsonpath={.status.conditions[?(@.type=='InputsValid')].status}")
				output, err := utils.Run(cmd)
				if err != nil {
					GinkgoWriter.Printf("Failed to get workload condition: %v\n", err)
					return false
				}

				status := string(output)
				GinkgoWriter.Printf("InputsValid condition status: %s\n", status)
				return status == "True"
			}, 60*time.Second, 5*time.Second).Should(BeTrue(), "InputsValid condition should become True")

			By("Verifying workload has been processed")
			cmd = exec.Command("kubectl", "get", "workload", "basic-web-test", "-n", "kbinit-system", "-o", "json")
			output, err := utils.Run(cmd)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("Workload status: %s\n", string(output))
		})
	})
})