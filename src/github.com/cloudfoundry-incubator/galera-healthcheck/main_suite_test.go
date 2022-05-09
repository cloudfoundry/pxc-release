package main_test

import (
	"github.com/onsi/gomega/gexec"
	"path"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo"
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
	Expect(binaryPath).To(BeAnExistingFile())
})

func getDirOfCurrentFile() string {
	_, filename, _, _ := runtime.Caller(1)
	return path.Dir(filename)
}

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})
