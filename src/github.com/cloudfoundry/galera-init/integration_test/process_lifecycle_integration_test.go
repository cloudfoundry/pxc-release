package integration_test

import (
	"database/sql"
	"os"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/internal/testing/docker"
)

var _ = Describe("galera-init integration", func() {
	var (
		baseCfg    config.Config
		scratchDir string
	)

	BeforeEach(func() {
		var err error
		scratchDir, err = os.MkdirTemp("", "galera-init")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { _ = os.RemoveAll(scratchDir) })

		baseCfg = config.Config{
			LogFileLocation: "/tmp/galera-init.log",
			Db: config.DBHelper{
				User:   "root",
				Socket: "/var/lib/mysql/mysql.sock",
			},
			Manager: config.StartManager{
				GaleraInitStatusServerAddress: "0.0.0.0:8114",
				StateFileLocation:             "/var/lib/mysql/node_state.txt",
				GrastateFileLocation:          "/var/lib/mysql/grastate.dat",
				ClusterIps: []string{
					"mysql0." + sessionID,
				},
				BootstrapNode:       true,
				ClusterProbeTimeout: 10,
			},
		}
	})

	When("Starting a single node", func() {
		var (
			galeraNode string
			db         *sql.DB
		)

		BeforeEach(func() {
			galeraNode = createGaleraContainer("mysql0", baseCfg)
			DeferCleanup(func() { _ = docker.RemoveContainer(galeraNode) })

			var err error
			db, err = docker.MySQLDB(galeraNode)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = db.Close() })
		})

		It("will allow MySQL to cleanly shutdown on SIGTERM", func() {
			Eventually(func() error {
				return serviceStatus(galeraNode)
			}, "3m", "1s").Should(Succeed())

			Expect(db.Ping()).To(Succeed(),
				`Expected MySQL instance to be reachable, but it was not`,
			)

			Expect(docker.Kill(galeraNode, "TERM")).To(Succeed())
			Expect(docker.WaitExited(galeraNode, 3*time.Minute)).To(Succeed())
			Expect(docker.Copy(galeraNode+":/var/log/mysql/mysql.err.log", scratchDir)).To(Succeed())

			Expect(scratchDir + "/mysql.err.log").To(BeAnExistingFile())

			contents, err := os.ReadFile(scratchDir + "/mysql.err.log")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(contents)).To(ContainSubstring("mysqld: Shutdown complete"))
		})

		It("will terminate with an error when mysql terminates ungracefully", func() {
			Eventually(
				func() error {
					return serviceStatus(galeraNode)
				}, "3m", "1s").Should(Succeed())

			Expect(db.Ping()).To(Succeed())

			Expect(docker.Kill(galeraNode, "KILL")).To(Succeed())

			exitCode, err := docker.ExitCode(galeraNode)
			Expect(err).NotTo(HaveOccurred())

			// exit due to signal will get exit code 128 + signum
			// 128 + 9 (SIGKILL) = 137
			const sigKillExitCode = "137"
			Expect(exitCode).To(Equal(sigKillExitCode))
		})
	})

	When("galera-init fails to bootstrap", func() {
		var (
			galeraNode string
		)

		BeforeEach(func() {
			baseCfg.Manager.ClusterIps = []string{
				"mysql0." + sessionID,
				"mysql1." + sessionID,
			}
			baseCfg.Manager.BootstrapNode = false

			galeraNode = createGaleraContainer("mysql0", baseCfg,
				AddEnvVars("INITIAL_CLUSTER_STATE=CLUSTERED"),
			)
		})

		It("exits with a non-zero status code", func() {
			Expect(docker.WaitExited(galeraNode, 3*time.Minute)).To(Succeed())

			exitCode, err := docker.ExitCode(galeraNode)
			Expect(err).NotTo(HaveOccurred())
			Expect(exitCode).To(Equal("1"),
				`Expected galera-init to exit with non-zero status, but it did not`)

			Expect(docker.Copy(galeraNode+":/var/log/mysql/mysql.err.log", scratchDir)).To(Succeed())

			Expect(scratchDir + "/mysql.err.log").To(BeAnExistingFile())

			contents, err := os.ReadFile(scratchDir + "/mysql.err.log")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(contents)).To(ContainSubstring("[WSREP] Provider/Node (gcomm://mysql0.%s) failed to establish connection with cluster", sessionID))
		})
	})

	When("galera-init orchestrates mysql joining an existing cluster", func() {
		var (
			galeraNode0 string
			galeraNode1 string
		)

		BeforeEach(func() {
			baseCfg.Manager.ClusterIps = []string{
				"mysql0." + sessionID,
				"mysql1." + sessionID,
			}
			wsrepClusterAddr := "gcomm://" + strings.Join(baseCfg.Manager.ClusterIps, ",")

			node0Cfg := baseCfg
			node0Cfg.Manager.BootstrapNode = true

			galeraNode0 = createGaleraContainer("mysql0", node0Cfg,
				AddEnvVars("WSREP_CLUSTER_ADDRESS="+wsrepClusterAddr),
			)
			DeferCleanup(func() { _ = docker.RemoveContainer(galeraNode0) })
			Eventually(
				func() error {
					return serviceStatus(galeraNode0)
				}, "3m", "1s").Should(Succeed())

			node1Cfg := baseCfg
			node1Cfg.Manager.BootstrapNode = false

			galeraNode1 = createGaleraContainer("mysql1", node1Cfg,
				AddEnvVars("WSREP_CLUSTER_ADDRESS="+wsrepClusterAddr),
			)
			DeferCleanup(func() {
				_ = docker.RemoveContainer(galeraNode1)
			})
		})

		It("should successfully join a second node to the cluster", func() {
			Eventually(
				func() error {
					return serviceStatus(galeraNode1)
				}, "3m", "1s").Should(Succeed())

			db, err := docker.MySQLDB(galeraNode1)
			Expect(err).NotTo(HaveOccurred())

			Expect(db.Ping()).To(Succeed())

			Eventually(func() (string, error) {
				var unused, wsrepClusterSize string
				wsrepClusterSizeQuery := `SHOW GLOBAL STATUS LIKE 'wsrep_cluster_size'`
				err := db.QueryRow(wsrepClusterSizeQuery).Scan(&unused, &wsrepClusterSize)
				return wsrepClusterSize, err
			}, "2m", "1s").Should(Equal("2"))
		})
	})
})
