package main_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func TestSwitchboard(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Switchboard Executable Suite")
}

var (
	switchboardBinPath string
)

var _ = BeforeSuite(func() {
	var err error
	switchboardBinPath, err = gexec.Build("github.com/cloudfoundry-incubator/switchboard/cmd/proxy")
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})
