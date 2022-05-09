package main_test

import (
	. "github.com/onsi/ginkgo"
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
})
