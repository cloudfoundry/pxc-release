package testhelper_test

import (
	"os"

	"github.com/cloudfoundry/pxc-release/replicator/testhelper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Testhelper/Helper", func() {
	It("starts testinstances with tls", func() {
		aliases := []string{"test"}
		net := testhelper.CreateTestNetwork()
		testhelper.StartContainerInstance("test", "test", "8.4", testhelper.VerifyCA, aliases, net)
	})
	It("leaves client key and cert empty on VERIFY_CA", func() {
		path, err := os.MkdirTemp("", "")
		Expect(err).ToNot(HaveOccurred())
		serverCerts, clientCerts := testhelper.InitCerts("test", path, testhelper.VerifyCA, []string{"alias"})

		Expect(clientCerts.PrivateKey).To(BeEmpty())
		Expect(clientCerts.Certificate).To(BeEmpty())
		Expect(clientCerts.CA).ToNot(BeEmpty())
		Expect(string(clientCerts.CA)).To(MatchRegexp("BEGIN CERTIFICATE"))
		Expect(string(clientCerts.CA)).To(ContainSubstring(string(serverCerts.CA)))
		// Expect(string(clientCerts.CA)).To(ContainSubstring(string(serverCerts.Certificate)))

		Expect(string(serverCerts.PrivateKey)).To(MatchRegexp("BEGIN RSA PRIVATE KEY"))
		Expect(string(serverCerts.CA)).To(MatchRegexp("BEGIN CERTIFICATE"))
		Expect(string(serverCerts.Certificate)).To(MatchRegexp("BEGIN CERTIFICATE"))
	})

	It("populates client key and cert on FULL", func() {
		path, err := os.MkdirTemp("", "")
		Expect(err).ToNot(HaveOccurred())
		serverCerts, clientCerts := testhelper.InitCerts("test", path, testhelper.VerifyIdentity, []string{"alias"})
		Expect(string(clientCerts.PrivateKey)).To(MatchRegexp("BEGIN RSA PRIVATE KEY"))
		Expect(string(serverCerts.PrivateKey)).To(MatchRegexp("BEGIN RSA PRIVATE KEY"))
		Expect(string(clientCerts.PrivateKey)).ToNot(Equal(serverCerts.PrivateKey))
		Expect(string(serverCerts.CA)).To(MatchRegexp("BEGIN CERTIFICATE"))
		Expect(string(clientCerts.Certificate)).To(MatchRegexp("BEGIN CERTIFICATE"))
		Expect(string(serverCerts.Certificate)).To(MatchRegexp("BEGIN CERTIFICATE"))
	})
})
