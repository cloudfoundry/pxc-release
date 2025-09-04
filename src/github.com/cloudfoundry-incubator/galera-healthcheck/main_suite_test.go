package main_test

import (
	"testing"

	"github.com/onsi/gomega/gexec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGaleraHealthcheck(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Galera Healthcheck Server Suite")
}

var (
	binaryPath string
)

var _ = BeforeSuite(func() {
	var err error

	binaryPath, err = gexec.Build("github.com/cloudfoundry-incubator/galera-healthcheck", "-race")
	Expect(err).ToNot(HaveOccurred())
	DeferCleanup(func() {
		gexec.CleanupBuildArtifacts()
	})
	Expect(binaryPath).To(BeAnExistingFile())
})
