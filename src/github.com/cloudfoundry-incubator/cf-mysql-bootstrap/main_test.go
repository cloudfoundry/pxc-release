package main_test

import (
	"fmt"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Bootstrap Executable", func() {
	AfterEach(func() {
		gexec.CleanupBuildArtifacts()
	})

	It("compiles the binary", func() {
		binaryPath, err := gexec.Build("github.com/cloudfoundry-incubator/cf-mysql-bootstrap")
		Expect(err).ToNot(HaveOccurred())
		Expect(binaryPath).To(BeAnExistingFile())
	})

	Describe("credential safety", func() {
		It("does not log the galera-agent password when bootstrap fails", func() {
			binaryPath, err := gexec.Build("github.com/cloudfoundry-incubator/cf-mysql-bootstrap")
			Expect(err).ToNot(HaveOccurred())

			const sensitivePassword = "s3cr3t-galera-agent-password"
			config := fmt.Sprintf(`{
				"HealthcheckURLs": ["http://127.0.0.1:19999"],
				"Username": "galera-agent",
				"Password": %q,
				"RepairMode": "bootstrap"
			}`, sensitivePassword)

			cmd := exec.Command(binaryPath, fmt.Sprintf("-config=%s", config))
			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).ToNot(HaveOccurred())

			Eventually(session, "10s").Should(gexec.Exit())
			Expect(session.ExitCode()).NotTo(Equal(0))

			combinedOutput := string(session.Out.Contents()) + string(session.Err.Contents())
			Expect(combinedOutput).NotTo(ContainSubstring(sensitivePassword),
				"galera-agent password must not appear in log output")
		})
	})
})
