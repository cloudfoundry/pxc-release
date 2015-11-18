package main_test

import (
	"io/ioutil"
	"os"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func TestBootstrap(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bootstrap Suite")
}

var (
	binaryPath string
	tempDir    string
)

var _ = BeforeSuite(func() {
	var err error
	binaryPath, err = gexec.Build("github.com/cloudfoundry-incubator/cf-mysql-bootstrap")
	Expect(err).ToNot(HaveOccurred())
	Expect(binaryPath).To(BeAnExistingFile())

	tempDir, err = ioutil.TempDir(os.TempDir(), "cf-mysql-bootstrap")
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
	os.RemoveAll(tempDir)
})
