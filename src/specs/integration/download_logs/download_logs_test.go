package download_logs_test

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("DownloadLogs", func() {
	var (
		logsDir string
	)
	BeforeEach(func() {
		var err error
		logsDir, err = ioutil.TempDir(os.TempDir(), "download_logs_")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		os.RemoveAll(logsDir)
	})

	It("fetches SHOW ENGINE INNODB STATUS output", func() {
		downloadLogsCmd := exec.Command("./download-logs", "-o", logsDir)
		downloadLogsCmd.Env = append(os.Environ(), "DOWNLOAD_LOGS_GPG_PASSPHRASE_FROM_STDIN=true")
		downloadLogsCmd.Dir = `../../../../scripts/`
		downloadLogsCmd.Stdin = bytes.NewBufferString("some-passphrase")

		session, err := gexec.Start(downloadLogsCmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())

		Eventually(session, "10m", "1s").Should(gexec.Exit(0))

		innodbStatusOutput := gbytes.NewBuffer()
		gpgCmd := fmt.Sprintf(`gpg -d --batch --passphrase=some-passphrase < %s/*-mysql-logs.tar.gz.gpg `+
			`| tar -Ozxv --wildcards "*/innodb_status.out"`, logsDir)
		decryptCmd := exec.Command("bash", "-c", gpgCmd)

		stdout := io.MultiWriter(GinkgoWriter, innodbStatusOutput)

		session, err = gexec.Start(decryptCmd, stdout, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())

		Eventually(session, "10m", "1s").Should(gexec.Exit(0))
		Expect(innodbStatusOutput).To(gbytes.Say(`(?m)^END OF INNODB MONITOR OUTPUT\s*$`))
	})
})
