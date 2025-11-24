package main_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var (
	pingdbPath string
)

func TestPingDB(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PingDB Suite")
}

var _ = BeforeSuite(func() {
	var err error
	pingdbPath, err = gexec.Build("github.com/cloudfoundry-incubator/switchboard/cmd/pingdb")
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	gexec.Kill()
	gexec.CleanupBuildArtifacts()
})
