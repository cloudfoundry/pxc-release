package main_test

import (
	"io"
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = XDescribe("binary", Ordered, func() {
	var binPath string

	BeforeAll(func() {
		var err error
		binPath, err = gexec.Build("setup-audit-log-filter-component")
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

		Expect(output).To(gbytes.Say("Something about missing MYSQL_DSN environment variable"))
	})
})
