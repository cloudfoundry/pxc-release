package integration_test

import (
	"database/sql"
	"strings"
	"syscall"

	"github.com/fsouza/go-dockerclient"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"github.com/cloudfoundry/galera-init/config"
	. "github.com/cloudfoundry/galera-init/integration_test/test_helpers"
)

var _ = Describe("galera-init integration", func() {
	var (
		baseCfg config.Config
	)

	BeforeEach(func() {
		baseCfg = config.Config{
			LogFileLocation: "/tmp/galera-init.log",
			Db: config.DBHelper{
				UpgradePath:        "mysql_upgrade",
				User:               "root",
				PreseededDatabases: nil,
				Socket:             "/var/run/mysqld/mysqld.sock",
			},
			Manager: config.StartManager{
				GaleraInitStatusServerAddress: "0.0.0.0:" + galeraInitStatusPort.Port(),
				StateFileLocation:             "/var/lib/mysql/node_state.txt",
				GrastateFileLocation:          "/var/lib/mysql/grastate.dat",
				ClusterIps: []string{
					"mysql0." + sessionID,
				},
				BootstrapNode:       true,
				ClusterProbeTimeout: 10,
			},
			Upgrader: config.Upgrader{
				PackageVersionFile:      "/tmp/VERSION",
				LastUpgradedVersionFile: "/var/lib/mysql/mysql_upgrade_info",
			},
		}
	})

	When("Starting a single node", func() {
		var (
			galeraNode *docker.Container
			db         *sql.DB
		)

		BeforeEach(func() {
			var err error

			galeraNode, err = createGaleraContainer("mysql0", baseCfg)
			Expect(err).NotTo(HaveOccurred())

			db, err = ContainerDBConnection(galeraNode, pxcMySQLPort)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if db != nil {
				_ = db.Close()
			}

			if galeraNode != nil {
				Expect(RemoveContainer(dockerClient, galeraNode)).To(Succeed())
			}
		})

		It("will allow MySQL to cleanly shutdown on SIGTERM", func() {
			StreamLogs(dockerClient, galeraNode)
			Eventually(func() error {
				return serviceStatus(galeraNode)
			}, "1m", "1s").Should(Succeed())

			Expect(db.Ping()).To(Succeed(),
				`Expected MySQL instance to be reachable, but it was not`,
			)

			Expect(dockerClient.KillContainer(docker.KillContainerOptions{
				ID:     galeraNode.ID,
				Signal: docker.SIGTERM,
			})).To(Succeed())

			Eventually(func() (isNotRunning bool, err error) {
				container, err := dockerClient.InspectContainer(galeraNode.ID)
				if err != nil {
					return false, err
				}
				return !container.State.Running, err
			}, "1m", "1s").Should(BeTrue())

			Expect(
				FetchContainerFileContents(
					dockerClient,
					galeraNode,
					"/var/log/mysql/mysql.err.log",
				),
			).To(HaveSuffix(`[Note] mysqld: Shutdown complete`))
		})

		It("will terminate with an error when mysql terminates ungracefully", func() {
			Eventually(
				func() error {
					return serviceStatus(galeraNode)
				}, "1m", "1s").Should(Succeed())

			Expect(db.Ping()).To(Succeed())

			_, err := HardKillMySQL(dockerClient, galeraNode)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() (exitCode int, err error) {
				container, err := dockerClient.InspectContainer(galeraNode.ID)
				if err != nil {
					return 0, err
				}
				return container.State.ExitCode, err
			}, "30s", "1s").Should(Equal(int(syscall.SIGKILL)))
		})
	})

	When("galera-init fails to bootstrap", func() {
		var (
			galeraNode *docker.Container
		)

		BeforeEach(func() {
			baseCfg.Manager.ClusterIps = []string{
				"mysql0." + sessionID,
				"mysql1." + sessionID,
			}
			baseCfg.Manager.BootstrapNode = false

			var err error
			galeraNode, err = createGaleraContainer("mysql0", baseCfg,
				AddEnvVars("INITIAL_CLUSTER_STATE=CLUSTERED"),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if galeraNode != nil {
				Expect(RemoveContainer(dockerClient, galeraNode)).To(Succeed())
			}
		})

		It("exits with a non-zero status code", func() {
			containerLogs := StreamLogs(dockerClient, galeraNode)

			Eventually(containerLogs, "5m", "1s").
				Should(gbytes.Say(`Mysqld exited with error; aborting. Review the mysqld error logs for more information.`))

			Eventually(func() (int, error) {
				container, err := dockerClient.InspectContainer(galeraNode.ID)
				if err != nil {
					return 0, err
				}
				return container.State.ExitCode, nil
			}, "30s", "1s").ShouldNot(BeZero())
		})
	})

	When("galera-init orchestrates mysql joining an existing cluster", func() {
		var (
			galeraNode0 *docker.Container
			galeraNode1 *docker.Container
		)

		BeforeEach(func() {
			baseCfg.Manager.ClusterIps = []string{
				"mysql0." + sessionID,
				"mysql1." + sessionID,
			}
			wsrepClusterAddr := "gcomm://" + strings.Join(baseCfg.Manager.ClusterIps, ",")

			var err error

			node0Cfg := baseCfg
			node0Cfg.Manager.BootstrapNode = true

			galeraNode0, err = createGaleraContainer("mysql0", node0Cfg,
				AddEnvVars("WSREP_CLUSTER_ADDRESS="+wsrepClusterAddr),
			)
			Expect(err).NotTo(HaveOccurred())
			Eventually(
				func() error {
					return serviceStatus(galeraNode0)
				}, "1m", "1s").Should(Succeed())

			node1Cfg := baseCfg
			node1Cfg.Manager.BootstrapNode = false

			galeraNode1, err = createGaleraContainer("mysql1", node1Cfg,
				AddEnvVars("WSREP_CLUSTER_ADDRESS="+wsrepClusterAddr),
			)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if galeraNode0 != nil {
				Expect(RemoveContainer(dockerClient, galeraNode0)).To(Succeed())
			}

			if galeraNode1 != nil {
				Expect(RemoveContainer(dockerClient, galeraNode1)).To(Succeed())
			}
		})

		It("should successfully join a second node to the cluster", func() {
			Eventually(
				func() error {
					return serviceStatus(galeraNode1)
				}, "1m", "1s").Should(Succeed())

			db, err := ContainerDBConnection(galeraNode1, pxcMySQLPort)
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
