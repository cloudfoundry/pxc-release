package main_test

import (
	"io"
	"os/exec"

	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("binary", Ordered, func() {
	var binPath string

	BeforeAll(func() {
		var err error
		binPath, err = gexec.Build("auditlogtools/cmd/configure-audit-log-component")
		Expect(err).NotTo(HaveOccurred())
	})

	It("requires the MYSQL_DSN environment variable", func() {
		output := gbytes.NewBuffer()
		cmd := exec.Command(binPath)
		cmd.Env = nil // Default
		cmd.Stdout = GinkgoWriter
		cmd.Stderr = io.MultiWriter(GinkgoWriter, output)

		err := cmd.Run()
		Expect(err).To(HaveOccurred())

		Expect(output).To(gbytes.Say(`required environment variable \\"MYSQL_DSN\\" is not set`))
	})

	It("requires the MYSQL_AUDIT_LOG_FILTER environment variable", func() {
		output := gbytes.NewBuffer()
		cmd := exec.Command(binPath)
		cmd.Env = nil // Default
		cmd.Stdout = GinkgoWriter
		cmd.Stderr = io.MultiWriter(GinkgoWriter, output)

		err := cmd.Run()
		Expect(err).To(HaveOccurred())

		Expect(output).To(gbytes.Say(`required environment variable \\"MYSQL_AUDIT_LOG_FILTER\\" is not set`))
	})
})
