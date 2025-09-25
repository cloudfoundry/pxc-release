package auditlogtools_test

import (
	"database/sql"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/onsi/gomega/gstruct"

	"auditlogtools/internal/testing/docker"
)

var _ = Describe("AuditLog Installation Workflow", Label("docker", "integration", "workflow"), func() {
	var (
		db             *sql.DB
		mysqlContainer string
	)

	var (
		once         sync.Once
		setupBinPath string
	)

	BeforeEach(func() {
		once.Do(func() {
			var err error
			setupBinPath, err = gexec.BuildWithEnvironment("auditlogtools/cmd/configure-audit-log-component", []string{"CGO_ENABLED=0"})
			Expect(err).NotTo(HaveOccurred())
		})

		var err error
		mysqlContainer, err = docker.RunContainer(docker.ContainerSpec{
			Image: "percona/percona-xtradb-cluster:8.4",
			// Note using pxc-main-transition-period=0 to speed up shutdowns in a testing context
			Args: []string{
				// Use GTIDs, so tests can more easily detect when transactions are written
				"--gtid-mode=on", "--enforce-gtid-consistency",
				// Disable Percona's maintenance transition period; This speeds up container startup by 10s
				"--pxc-maint-transition-period=0",
				// Specify some basic audit log configuration
				"--loose-audit_log_filter.format=JSON",
				"--loose-audit_log_filter.file=/logs/audit.log",
			},
			Env:            []string{"MYSQL_ALLOW_EMPTY_PASSWORD=1", "PXC_CLUSTER_NAME=galera"},
			Ports:          []string{"3306/tcp"},
			HealthCmd:      "mysqladmin -u root --host=127.0.0.1 ping",
			HealthInterval: "1s",
			Volumes:        []string{"/logs"},
		})
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(docker.RemoveContainer(mysqlContainer)).To(Succeed()) })
		Expect(docker.WaitHealthy(mysqlContainer, 5*time.Minute)).To(Succeed())

		db, err = docker.MySQLDB(mysqlContainer)
		Expect(err).NotTo(HaveOccurred())
		Expect(db.Ping()).To(Succeed())
	})

	runSetup := func() {
		GinkgoHelper()

		cmd := exec.Command(setupBinPath)
		cmd.Env = []string{
			// NOTE: Some of the Percona Audit Log UDFs do NOT support prepared statements
			//       Use "interpolateParams" to prepare client side; This will be a requirement for an actual deployment as well.
			"MYSQL_DSN=root@tcp(127.0.0.1:" + docker.ContainerPort(mysqlContainer, "3306/tcp") + ")/mysql?interpolateParams=true",
			// TODO: Test user@% with a wildcard host when Percona XtraDB Cluster v8.4.4+ is available
			// See: https://perconadev.atlassian.net/browse/PS-9024
			"MYSQL_AUDIT_EXCLUDE_USERS=user1@localhost,user2@localhost",
		}
		cmd.Stdout = GinkgoWriter
		cmd.Stderr = GinkgoWriter
		Expect(cmd.Run()).To(Succeed())
	}

	It("enables audit logging", func() {
		runSetup()

		Expect(db.Exec(`SELECT audit_log_rotate()`)).Error().NotTo(HaveOccurred())

		// TODO: Validate via SELECT audit_read_log
		//       Currently this seems to always return [null] for me
		//       Maybe a Percona bug, maybe a configuration issue or maybe a misunderstanding of the feature
		t, err := os.MkdirTemp("", "auditlog-test")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(func() { Expect(os.RemoveAll(t)).To(Succeed()) })

		Expect(docker.Copy(mysqlContainer+":/logs/.", t)).To(Succeed())

		paths, err := filepath.Glob(filepath.Join(t, "/audit[.]*[.]log"))
		Expect(err).NotTo(HaveOccurred())
		Expect(paths).To(HaveLen(1))

		Expect(paths[0]).To(BeARegularFile())
		logContent, err := os.ReadFile(paths[0])
		Expect(err).NotTo(HaveOccurred())

		var structuredLog []map[string]any
		Expect(json.Unmarshal(logContent, &structuredLog)).To(Succeed())
		Expect(structuredLog).To(ContainElement(gstruct.MatchAllKeys(gstruct.Keys{
			"timestamp": MatchRegexp(`\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`),
			"id":        BeNumerically(">=", 0),
			"class":     Equal("audit"),
			"event":     Equal("audit"),
			"server_id": BeNumerically(">=", 1),
		})))
	})

	When("running audit log filter setup repeatedly", func() {
		BeforeEach(func() {
			conn, err := docker.MySQLDB(mysqlContainer)
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				Expect(conn.Exec(`SET SESSION wsrep_on = on`)).Error().NotTo(HaveOccurred())
				Expect(conn.Close()).To(Succeed())
			}()

			Expect(conn.Exec(`SET SESSION wsrep_on = off`)).Error().NotTo(HaveOccurred())
			Expect(conn.Exec(`RESET BINARY LOGS AND GTIDS`)).Error().NotTo(HaveOccurred())
		})

		It("idempotently enables audit logging", func() {
			var initialGTID string
			Expect(db.QueryRow(`SELECT @@global.gtid_executed`).Scan(&initialGTID)).To(Succeed())
			Expect(initialGTID).To(BeEmpty(), `Expected initial state to start with zero transactions`)

			runSetup()

			var preGTIDExecuted string
			Expect(db.QueryRow(`SELECT @@global.gtid_executed`).Scan(&preGTIDExecuted)).To(Succeed())
			Expect(preGTIDExecuted).ToNot(BeEmpty(), `Expected initial setup to write to the database`)

			runSetup()

			var postGTIDExecuted string
			Expect(db.QueryRow(`SELECT @@global.gtid_executed`).Scan(&postGTIDExecuted)).To(Succeed())
			Expect(postGTIDExecuted).To(Equal(preGTIDExecuted),
				`Expected repeated setup to be idempotent and perform no changes to the database`)
		})
	})
})
