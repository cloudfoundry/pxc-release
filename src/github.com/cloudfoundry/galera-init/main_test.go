package main_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Executable", func() {

	It("compiles the binary without errors", func() {
		_, err := gexec.Build("github.com/cloudfoundry-incubator/switchboard/")
		gexec.CleanupBuildArtifacts()
		Expect(err).NotTo(HaveOccurred())
	})
})
