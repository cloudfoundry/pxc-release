package testhelper_test

import (
	"os"
	"time"

	"github.com/cloudfoundry/pxc-release/replicator/testhelper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
)

var _ = Describe("Testhelper/Helper", func() {
	var container *testcontainers.DockerContainer
	It("starts testinstances with tls", func() {
		aliases := []string{"test"}
		net := testhelper.CreateTestNetwork()
		_, _, container = testhelper.StartPXCInstance("test", "8.4", testhelper.VerifyCA, aliases, net)
		testcontainers.CleanupContainer(GinkgoTB(), container, testcontainers.StopTimeout(120*time.Second))
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
