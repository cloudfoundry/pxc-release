package no_remote_access_test

import (
	"os"
	"os/exec"

	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func disableRemoteAdminAccess() {
	tmp, err := os.CreateTemp("", os.Getenv("BOSH_DEPLOYMENT") + "bosh-manifest.yml")
	Expect(err).NotTo(HaveOccurred())

	defer os.Remove(tmp.Name())

	cmd := exec.Command("bosh", "manifest")
	cmd.Stdout = tmp
	cmd.Stderr = GinkgoWriter
	Expect(cmd.Run()).To(Succeed())

	cmd = exec.Command("bosh", "deploy", tmp.Name(),
		"-o", "../../../../operations/disable-remote-admin-access.yml", "-n", "--tty")
	cmd.Stderr = GinkgoWriter
	cmd.Stdout = GinkgoWriter
	Expect(cmd.Run()).To(Succeed())
}

var _ = Describe("CF PXC No Remote Admin Access", func() {
	BeforeEach(func() {
		disableRemoteAdminAccess()
	})
	It("does not allow access to mysql from anywhere besides localhost", func() {
		query := "show variables"
		_, err := mysqlConn.Query(query)
		Expect(err).To(MatchError(MatchRegexp("is not allowed to connect to this MySQL server|Access denied for user")))
	})
})
