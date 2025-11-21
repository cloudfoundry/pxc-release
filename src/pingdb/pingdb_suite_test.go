package main_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
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
	pingdbPath, err = gexec.Build("pingdb")
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	gexec.Kill()
	gexec.CleanupBuildArtifacts()
})
