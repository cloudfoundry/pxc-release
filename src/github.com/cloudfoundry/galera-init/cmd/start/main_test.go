package main_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("galera-init Start", func() {
	Describe("Executable", func() {
		It("compiles the binary without errors", func() {
			_, err := gexec.Build("github.com/cloudfoundry/galera-init/cmd/start")
			gexec.CleanupBuildArtifacts()
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
