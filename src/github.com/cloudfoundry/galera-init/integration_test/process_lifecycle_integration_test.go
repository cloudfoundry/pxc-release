package integration_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/integration_test/docker"
)

var _ = Describe("galera-init integration", func() {
	var (
		baseCfg config.Config
	)

	BeforeEach(func() {
		baseCfg = config.Config{
			LogFileLocation: "/tmp/galera-init.log",
			Db: config.DBHelper{
				User:   "root",
				Socket: "/var/lib/mysql/mysql.sock",
			},
			Manager: config.StartManager{
				GaleraInitStatusServerAddress: "0.0.0.0:" + "8114",
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
			galeraContainer string
			db              *sql.DB
		)

		BeforeEach(func() {
			var err error

			galeraContainer, err = createGaleraContainer("mysql0", baseCfg)
			Expect(err).NotTo(HaveOccurred())

			Expect(docker.WaitHealthy(galeraContainer, 5*time.Minute)).To(Succeed())

			db, err = docker.MySQLDB(galeraContainer)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if db != nil {
				_ = db.Close()
			}

			if galeraContainer != "" {
				Expect(docker.RemoveContainer(galeraContainer)).To(Succeed())
			}
		})

		It("will allow MySQL to cleanly shutdown on SIGTERM", func() {
			Eventually(func() error {
				return serviceStatus(galeraContainer)
			}, "3m", "1s").Should(Succeed())

			Expect(db.Ping()).To(Succeed(),
				`Expected MySQL instance to be reachable, but it was not`,
			)

			Expect(docker.Kill(galeraContainer, "SIGTERM")).To(Succeed())

			Eventually(func() (isNotRunning bool, err error) {
				status, err := docker.InspectStatus(galeraContainer)
				if err != nil {
					return false, err
				}
				return status != "running", err
			}, "3m", "1s").Should(BeTrue())

			tempDir, err := os.MkdirTemp("", "logs_")
			Expect(err).NotTo(HaveOccurred())
			errorLog := filepath.Join(tempDir, "mysql.err.log")
			err = docker.Copy(galeraContainer+":/var/log/mysql/mysql.err.log", errorLog)
			Expect(err).NotTo(HaveOccurred())
			mysqlErrLogContents, err := os.ReadFile(errorLog)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(mysqlErrLogContents)).To(ContainSubstring("mysqld: Shutdown complete"))
		})

		It("will terminate with an error when mysql terminates ungracefully", func() {
			Eventually(func() error {
				return serviceStatus(galeraContainer)
			}, "3m", "1s").Should(Succeed())

			Expect(db.Ping()).To(Succeed())

			err := docker.HardKillMySQL(galeraContainer)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() (value int, err error) {
				exitCode, err := docker.InspectExitCode(galeraContainer)
				if err != nil {
					return 0, err
				}
				return exitCode, err
			}, "30s", "1s").Should(Equal(int(syscall.SIGKILL)))
		})
	})

	When("galera-init fails to bootstrap", func() {
		var galeraContainer string

		BeforeEach(func() {
			baseCfg.Manager.ClusterIps = []string{
				"mysql0." + sessionID,
				"mysql1." + sessionID,
			}
			baseCfg.Manager.BootstrapNode = false

			var err error
			galeraContainer, err = createGaleraContainer("mysql0", baseCfg, "INITIAL_CLUSTER_STATE=CLUSTERED")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if galeraContainer != "" {
				Expect(docker.RemoveContainer(galeraContainer)).To(Succeed())
			}
		})

		It("exits with a non-zero status code", func() {
			Eventually(func() (bool, error) {
				var err error
				status, err := docker.InspectStatus(galeraContainer)
				if err != nil {
					return true, err
				}
				return status == "running", nil
			}, "3m", "1s").ShouldNot(BeTrue())

			exitCode, err := docker.InspectExitCode(galeraContainer)
			Expect(err).NotTo(HaveOccurred())
			Expect(exitCode).ToNot(BeZero())

			tempDir, err := os.MkdirTemp("", "logs_")
			Expect(err).NotTo(HaveOccurred())
			errorLog := filepath.Join(tempDir, "mysql.err.log")
			err = docker.Copy(galeraContainer+":/var/log/mysql/mysql.err.log", errorLog)
			Expect(err).NotTo(HaveOccurred())
			mysqlErrLogContents, err := os.ReadFile(errorLog)
			Expect(err).NotTo(HaveOccurred())

			Expect(string(mysqlErrLogContents)).To(
				ContainSubstring(
					`[WSREP] Provider/Node (gcomm://mysql0.%s) failed to establish connection with cluster`,
					sessionID,
				),
			)
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

			var err error
			galeraNode0, err = createGaleraContainer("mysql0", node0Cfg, "WSREP_CLUSTER_ADDRESS="+wsrepClusterAddr)
			DeferCleanup(func() { _ = docker.RemoveContainer(galeraNode0) })
			Expect(err).NotTo(HaveOccurred())
			Expect(docker.WaitHealthy(galeraNode0, 5*time.Minute)).To(Succeed())
			Eventually(func() error {
				return serviceStatus(galeraNode0)
			}, "3m", "1s").Should(Succeed())

			node1Cfg := baseCfg
			node1Cfg.Manager.BootstrapNode = false

			galeraNode1, err = createGaleraContainer("mysql1", node1Cfg, "WSREP_CLUSTER_ADDRESS="+wsrepClusterAddr)
			DeferCleanup(func() { _ = docker.RemoveContainer(galeraNode1) })
			Expect(docker.WaitHealthy(galeraNode1, 5*time.Minute)).To(Succeed())
			Expect(err).NotTo(HaveOccurred())
		})

		It("should successfully join a second node to the cluster", func() {
			Eventually(func() error {
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
