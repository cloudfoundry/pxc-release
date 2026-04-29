package main_test

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"code.cloudfoundry.org/tlsconfig"
	"code.cloudfoundry.org/tlsconfig/certtest"
	"gopkg.in/yaml.v3"

	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/test_helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Sequence Number during SST", func() {
	var (
		tlsClientConfig *tls.Config
		galeraAgentPort int
		dataDir         string
		sentinelFile    string
		stubMysqldPath  string
		agentCfg        config.Config
	)

	BeforeEach(func() {
		// TLS setup: one CA signs the server cert; the client trusts that CA.
		authority, err := certtest.BuildCA("serverCA")
		Expect(err).ToNot(HaveOccurred())

		serverCert, err := authority.BuildSignedCertificate("serverCert")
		Expect(err).ToNot(HaveOccurred())

		serverCertPEM, serverKeyPEM, err := serverCert.CertificatePEMAndPrivateKey()
		Expect(err).ToNot(HaveOccurred())

		pool, err := authority.CertPool()
		Expect(err).ToNot(HaveOccurred())

		tlsClientConfig, err = tlsconfig.Build(
			tlsconfig.WithInternalServiceDefaults(),
		).Client(tlsconfig.WithAuthority(pool))
		Expect(err).ToNot(HaveOccurred())

		// Temp data directory. Both contexts place grastate.dat here (it is
		// preserved by the SST cleanup cpat pattern and is present throughout SST),
		// and use the presence/absence of the sst_in_progress file — written by
		// Galera when it invokes the SST script — as the SST indicator.
		dataDir = GinkgoT().TempDir()
		sentinelFile = filepath.Join(dataDir, "wsrep-recover-was-called")

		// grastate.dat is always present: the xtrabackup-v2 SST cpat pattern
		// explicitly preserves it during the data directory cleanup phase.
		grastateContents := "# GALERA saved state\nversion: 2.1\ngroup_id: test-uuid\nuuid: test-uuid\nseqno: 42\n"
		Expect(os.WriteFile(
			filepath.Join(dataDir, "grastate.dat"),
			[]byte(grastateContents),
			0644,
		)).To(Succeed())

		// Stub mysqld: touch a sentinel file so tests can detect invocation.
		// RecoverSeqno() passes --log-error=<path> and reads that file for the
		// "WSREP: Recovered position:" line, so the stub must write to that path.
		stubScript := fmt.Sprintf(`#!/bin/bash
touch %q
for arg in "$@"; do
  case "$arg" in
    --log-error=*)
      LOG_ERROR_FILE="${arg#--log-error=}"
      ;;
  esac
done
if [ -n "$LOG_ERROR_FILE" ]; then
  echo "WSREP: Recovered position: test-uuid:42" >> "$LOG_ERROR_FILE"
fi
exit 0
`, sentinelFile)

		f, err := os.CreateTemp("", "mysqld-sst-stub-*.sh")
		Expect(err).NotTo(HaveOccurred())
		_, err = f.WriteString(stubScript)
		Expect(err).NotTo(HaveOccurred())
		Expect(f.Close()).To(Succeed())
		Expect(os.Chmod(f.Name(), 0755)).To(Succeed())
		stubMysqldPath = f.Name()
		DeferCleanup(func() { os.Remove(stubMysqldPath) })

		galeraAgentPort = test_helpers.RandomPort()

		agentCfg = config.Config{
			DataDir: dataDir,
			DB: config.DBConfig{
				// Non-existent socket ensures dbReachable() returns false,
				// landing execution on the wsrep-recover code path.
				Socket:   filepath.Join(dataDir, "nonexistent.sock"),
				Password: "root-password",
			},
			GaleraInit: config.GaleraInitConfig{
				MysqlStateFilePath:            filepath.Join(dataDir, "state.txt"),
				ServiceName:                   "galera-init",
				GaleraInitStatusServerAddress: "127.0.0.1:9999",
			},
			Host:       "localhost",
			Port:       galeraAgentPort,
			MysqldPath: stubMysqldPath,
			MyCnfPath:  filepath.Join(dataDir, "my.cnf"),
			SidecarEndpoint: config.SidecarEndpointConfig{
				Username: "basic-auth-username",
				Password: "basic-auth-password",
				TLS: config.EndpointTLS{
					Enabled:     true,
					Certificate: string(serverCertPEM),
					PrivateKey:  string(serverKeyPEM),
				},
			},
		}
	})

	// startAgent compiles agentCfg to YAML and launches the binary, then waits
	// for the /health endpoint to become responsive. It registers a DeferCleanup
	// that kills only this specific session, not all gexec sessions globally.
	startAgent := func() {
		b, err := yaml.Marshal(&agentCfg)
		Expect(err).NotTo(HaveOccurred())

		cmd := exec.Command(binaryPath, fmt.Sprintf("-config=%s", string(b)))
		session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { session.Kill().Wait(time.Minute) })

		healthClient := &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsClientConfig},
			Timeout:   5 * time.Second,
		}
		Eventually(func() error {
			_, err := healthClient.Get(fmt.Sprintf("https://127.0.0.1:%d/health", galeraAgentPort))
			return err
		}, "10s", "200ms").Should(Succeed())
	}

	getSequenceNumber := func() *http.Response {
		client := &http.Client{
			Transport: &http.Transport{TLSClientConfig: tlsClientConfig},
			Timeout:   15 * time.Second,
		}
		req, err := http.NewRequest("GET",
			fmt.Sprintf("https://127.0.0.1:%d/sequence_number", galeraAgentPort), nil)
		Expect(err).NotTo(HaveOccurred())
		req.SetBasicAuth("basic-auth-username", "basic-auth-password")

		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	Describe("GET /sequence_number", func() {
		Context("when SST is in progress (sst_in_progress file is present in the data directory)", func() {
			JustBeforeEach(func() {
				// sst_in_progress is written by Galera when it invokes the SST
				// script. The xtrabackup-v2 cpat pattern preserves it during the
				// data directory cleanup, so it remains present for the duration
				// of the transfer. grastate.dat is also preserved by cpat and is
				// present throughout — it cannot be used to detect SST.
				Expect(os.WriteFile(
					filepath.Join(dataDir, "sst_in_progress"),
					[]byte{},
					0644,
				)).To(Succeed())

				startAgent()
			})

			It("returns an error and does NOT invoke mysqld --wsrep-recover", func() {
				resp := getSequenceNumber()
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)

				Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError),
					"expected HTTP 500 while SST is in progress, got %d: %s", resp.StatusCode, string(body))

				Expect(sentinelFile).NotTo(BeAnExistingFile(),
					"mysqld --wsrep-recover must NOT be invoked while SST is in progress")
			})
		})

		Context("when SST is not in progress (sst_in_progress file is absent)", func() {
			JustBeforeEach(func() {
				// No sst_in_progress file — normal stopped-node state.
				startAgent()
			})

			It("invokes mysqld --wsrep-recover and returns the sequence number", func() {
				resp := getSequenceNumber()
				defer resp.Body.Close()
				body, _ := io.ReadAll(resp.Body)

				Expect(resp.StatusCode).To(Equal(http.StatusOK),
					"expected HTTP 200 when SST is not in progress, got %d: %s", resp.StatusCode, string(body))

				Expect(sentinelFile).To(BeAnExistingFile(),
					"mysqld --wsrep-recover should have been invoked when SST is not in progress")
			})
		})
	})
})
