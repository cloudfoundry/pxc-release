package testhelper_test

import (
	"os"

	"github.com/cloudfoundry/pxc-release/replicator/testhelper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Testhelper/Helper", func() {
	It("starts testinstances with tls", func() {
		net, aliases := testhelper.CreateTestNetwork()
		testhelper.StartContainerInstance("test", "test", "8.4", true, aliases, net)
	})
	It("generates certs", func() {
		path, err := os.MkdirTemp("", "")
		Expect(err).ToNot(HaveOccurred())
		serverCerts, clientCerts := testhelper.InitCerts("test", path, []string{"alias"})

		Expect(clientCerts.PrivateKey).To(MatchRegexp("BEGIN RSA PRIVATE KEY"))
		Expect(serverCerts.PrivateKey).To(MatchRegexp("BEGIN RSA PRIVATE KEY"))
		Expect(clientCerts.PrivateKey).ToNot(Equal(serverCerts.PrivateKey))
		Expect(serverCerts.CA).To(MatchRegexp("BEGIN CERTIFICATE"))
		Expect(serverCerts.CA).To(Equal(clientCerts.CA))
		Expect(clientCerts.Certificate).To(MatchRegexp("BEGIN CERTIFICATE"))
		Expect(serverCerts.Certificate).To(MatchRegexp("BEGIN CERTIFICATE"))
	})
})
