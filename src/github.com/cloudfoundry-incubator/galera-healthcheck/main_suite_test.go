package main_test

import (
	"net"
	"strconv"
	"testing"

	"github.com/onsi/gomega/gexec"

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

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})

func randomPort() int {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	Expect(err).NotTo(HaveOccurred())

	_, port, err := net.SplitHostPort(l.Addr().String())
	Expect(err).NotTo(HaveOccurred())

	intPort, err := strconv.Atoi(port)
	Expect(err).NotTo(HaveOccurred())

	return intPort
}
