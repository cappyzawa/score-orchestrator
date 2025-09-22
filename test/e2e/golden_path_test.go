package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cappyzawa/score-orchestrator/test/utils"
)

var _ = Describe("Golden Path E2E Test", func() {
	var (
		namespaceName string
		workloadFile  string
	)

	BeforeEach(func() {
		// Create a unique namespace for this test
		namespaceName = fmt.Sprintf("e2e-golden-path-%d", time.Now().UnixNano())

		// Create namespace
		cmd := exec.Command("kubectl", "create", "namespace", namespaceName)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		// Create workload YAML content (Workload CRD format based on user's Score spec)
		workloadContent := fmt.Sprintf(`apiVersion: score.dev/v1b1
kind: Workload
metadata:
  name: service-a
  namespace: %s
spec:
  service:
    ports:
    - port: 8000
      targetPort: 80
  containers:
    container-id:
      image: nginx:alpine
      resources:
        requests:
          cpu: "100m"
          memory: "64Mi"
        limits:
          cpu: "200m"
          memory: "128Mi"
      variables:
        CONNECTION_STRING: >-
          postgresql://$${resources.db.username}:$${resources.db.password}@$${resources.db.host}:$${resources.db.port}
  resources:
    db:
      type: postgres
`, namespaceName)

		// Write workload to temporary file
		workloadFile = fmt.Sprintf("/tmp/workload-%s.yaml", namespaceName)
		err = os.WriteFile(workloadFile, []byte(workloadContent), 0644)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		// Clean up the namespace
		if namespaceName != "" {
			cmd := exec.Command("kubectl", "delete", "namespace", namespaceName, "--ignore-not-found=true")
			_, _ = utils.Run(cmd)
		}
		// Clean up temporary file
		if workloadFile != "" {
			_ = exec.Command("rm", "-f", workloadFile).Run()
		}
	})

	It("should complete the golden path from Workload to Ready state", func() {
		By("Creating the Workload")
		cmd := exec.Command("kubectl", "apply", "-f", workloadFile)
		_, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())

		By("Waiting for ResourceClaim to be created")
		Eventually(func() bool {
			cmd := exec.Command("kubectl", "get", "resourceclaim.score.dev", "service-a-db", "-n", namespaceName, "-o", "name")
			_, err := utils.Run(cmd)
			return err == nil
		}, time.Minute, time.Second).Should(BeTrue())

		By("Verifying ResourceClaim type")
		cmd = exec.Command("kubectl", "get", "resourceclaim.score.dev", "service-a-db", "-n", namespaceName,
			"-o", "jsonpath={.spec.type}")
		output, err := utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		Expect(strings.TrimSpace(output)).To(Equal("postgres"))

		By("Waiting for ResourceClaim to reach Bound phase")
		Eventually(func() string {
			cmd := exec.Command("kubectl", "get", "resourceclaim.score.dev", "service-a-db", "-n", namespaceName,
				"-o", "jsonpath={.status.phase}")
			output, err := utils.Run(cmd)
			if err != nil {
				return ""
			}
			return strings.TrimSpace(output)
		}, time.Minute*2, time.Second*5).Should(Equal("Bound"))

		By("Verifying ResourceClaim outputs available")
		Eventually(func() string {
			cmd := exec.Command("kubectl", "get", "resourceclaim.score.dev", "service-a-db", "-n", namespaceName,
				"-o", "jsonpath={.status.outputsAvailable}")
			output, err := utils.Run(cmd)
			if err != nil {
				return ""
			}
			return strings.TrimSpace(output)
		}, time.Minute, time.Second*5).Should(Equal("true"))

		By("Verifying the generated Secret exists")
		Eventually(func() bool {
			cmd := exec.Command("kubectl", "get", "secret", "service-a-db-postgres-secret", "-n", namespaceName, "-o", "name")
			_, err := utils.Run(cmd)
			return err == nil
		}, time.Minute, time.Second).Should(BeTrue())

		By("Verifying Secret contains expected keys")
		cmd = exec.Command("kubectl", "get", "secret", "service-a-db-postgres-secret", "-n", namespaceName,
			"-o", "jsonpath={.data}")
		output, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		secretData := output
		Expect(secretData).To(ContainSubstring("username"))
		Expect(secretData).To(ContainSubstring("password"))
		Expect(secretData).To(ContainSubstring("host"))
		Expect(secretData).To(ContainSubstring("port"))
		Expect(secretData).To(ContainSubstring("database"))
		Expect(secretData).To(ContainSubstring("uri"))

		By("Waiting for WorkloadPlan to be created")
		Eventually(func() bool {
			cmd := exec.Command("kubectl", "get", "workloadplan", "service-a", "-n", namespaceName, "-o", "name")
			_, err := utils.Run(cmd)
			return err == nil
		}, time.Minute, time.Second).Should(BeTrue())

		By("Waiting for Kubernetes Deployment to be created by runtime controller")
		Eventually(func() bool {
			cmd := exec.Command("kubectl", "get", "deployment", "service-a", "-n", namespaceName, "-o", "name")
			_, err := utils.Run(cmd)
			return err == nil
		}, time.Minute, time.Second).Should(BeTrue())

		By("Waiting for Kubernetes Service to be created by runtime controller")
		Eventually(func() bool {
			cmd := exec.Command("kubectl", "get", "service", "service-a", "-n", namespaceName, "-o", "name")
			_, err := utils.Run(cmd)
			return err == nil
		}, time.Minute, time.Second).Should(BeTrue())

		By("Verifying Deployment has correct resource requirements")
		cmd = exec.Command("kubectl", "get", "deployment", "service-a", "-n", namespaceName,
			"-o", "jsonpath={.spec.template.spec.containers[0].resources}")
		output, err = utils.Run(cmd)
		Expect(err).NotTo(HaveOccurred())
		resourcesOutput := strings.TrimSpace(output)
		Expect(resourcesOutput).To(ContainSubstring("100m"))  // CPU request
		Expect(resourcesOutput).To(ContainSubstring("64Mi"))  // Memory request
		Expect(resourcesOutput).To(ContainSubstring("200m"))  // CPU limit
		Expect(resourcesOutput).To(ContainSubstring("128Mi")) // Memory limit

		By("Waiting for Deployment to be ready")
		Eventually(func() bool {
			cmd := exec.Command("kubectl", "get", "deployment", "service-a", "-n", namespaceName,
				"-o", "jsonpath={.status.readyReplicas}")
			output, err := utils.Run(cmd)
			if err != nil {
				return false
			}
			readyReplicas := strings.TrimSpace(output)
			return readyReplicas == "1"
		}, time.Minute*2, time.Second*5).Should(BeTrue())

		By("Verifying Workload Ready condition - the ultimate goal")
		Eventually(func() string {
			cmd := exec.Command("kubectl", "get", "workload", "service-a", "-n", namespaceName,
				"-o", "jsonpath={.status.conditions[?(@.type==\"Ready\")].status}")
			output, err := utils.Run(cmd)
			if err != nil {
				return ""
			}
			readyStatus := strings.TrimSpace(output)

			// Log current status if not ready for debugging
			if readyStatus != "True" {
				// Check individual conditions for troubleshooting
				inputsValidCmd := exec.Command("kubectl", "get", "workload", "service-a", "-n", namespaceName,
					"-o", "jsonpath={.status.conditions[?(@.type==\"InputsValid\")].status}")
				inputsValidOutput, _ := utils.Run(inputsValidCmd)

				claimsReadyCmd := exec.Command("kubectl", "get", "workload", "service-a", "-n", namespaceName,
					"-o", "jsonpath={.status.conditions[?(@.type==\"ClaimsReady\")].status}")
				claimsReadyOutput, _ := utils.Run(claimsReadyCmd)

				runtimeReadyCmd := exec.Command("kubectl", "get", "workload", "service-a", "-n", namespaceName,
					"-o", "jsonpath={.status.conditions[?(@.type==\"RuntimeReady\")].status}")
				runtimeReadyOutput, _ := utils.Run(runtimeReadyCmd)

				By(fmt.Sprintf("Workload conditions - Ready: %s, InputsValid: %s, ClaimsReady: %s, RuntimeReady: %s",
					readyStatus, strings.TrimSpace(inputsValidOutput),
					strings.TrimSpace(claimsReadyOutput), strings.TrimSpace(runtimeReadyOutput)))
			}

			return readyStatus
		}, time.Minute*3, time.Second*5).Should(Equal("True"))
	})
})
