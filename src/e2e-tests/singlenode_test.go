package e2e_tests

import (
	"bytes"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
	"e2e-tests/utilities/cmd"
)

var _ = Describe(fmt.Sprintf("Single Node for PXC version %s", expectedMysqlVersion), Ordered, Label("single-node"), func() {
	var (
		db             *sql.DB
		deploymentName string
	)

	BeforeAll(func() {
		deploymentName = "pxc-single-node-" + uuid.New().String()

		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation(`test/seed-test-user.yml`),
		)).To(Succeed())

		// Skip running smoke tests for this test group because smoke-test write to the database and generates GTIDs,
		// violating an assumption of this test.

		mysqlIPs, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("mysql"))
		Expect(err).NotTo(HaveOccurred())
		Expect(mysqlIPs).To(HaveLen(1))
		mysqlHost := mysqlIPs[0]

		db, err = sql.Open("mysql", "test-admin:integration-tests@tcp("+mysqlHost+")/?tls=skip-verify&interpolateParams=true")
		Expect(err).NotTo(HaveOccurred())
		db.SetMaxIdleConns(0)
		db.SetMaxOpenConns(1)
	})

	AfterAll(func() {
		if CurrentSpecReport().Failed() {
			return
		}

		Expect(bosh.DeleteDeployment(deploymentName)).To(Succeed())
	})

	It("runs expected Percona XtraDB Cluster version", func() {
		var mysqlVersion string
		Expect(db.QueryRow(`SELECT @@global.version`).Scan(&mysqlVersion)).To(Succeed())
		Expect(mysqlVersion).To(HavePrefix(expectedMysqlVersion))
	})

	It("has an empty GTID transaction history on startup", func() {
		var queryResultString string
		Expect(db.QueryRow("SELECT @@global.gtid_executed;").Scan(&queryResultString)).
			To(Succeed())
		Expect(queryResultString).To(BeEmpty())

		Expect(db.Exec(`CREATE DATABASE binary_logs`)).
			Error().NotTo(HaveOccurred())

		Expect(db.QueryRow("SELECT @@global.gtid_executed;").Scan(&queryResultString)).
			To(Succeed())
		Expect(queryResultString).ToNot(BeEmpty())
	})
	It("does not go through crash recovery", func() {
		output, err := bosh.RemoteCommand(deploymentName, "mysql/0", "stat /var/vcap/sys/log/pxc-mysql/pxc-57-recovery.log")
		Expect(output).To(ContainSubstring(`stat: cannot statx '/var/vcap/sys/log/pxc-mysql/pxc-57-recovery.log': No such file or directory`))
		Expect(err).To(HaveOccurred())
	})
	It("does not enable jemalloc", Label("jemalloc"), func() {
		// When jemalloc is disabled the Percona provided P_S.malloc_* tables should report "0" counters
		var jemallocAllocated uint64
		Expect(db.QueryRow(`SELECT ALLOCATED FROM performance_schema.malloc_stats_totals`).
			Scan(&jemallocAllocated)).To(Succeed())

		Expect(jemallocAllocated).To(BeZero(),
			`Expected to observe no recorded allocations for jemalloc, but malloc_stats_totals was non-zero!`)

		// Additionally, for thoroughness, check that jemalloc is not reported in the memory map of the mysqld process
		var output bytes.Buffer
		err := cmd.RunWithoutOutput(&output,
			"bosh",
			"--deployment="+deploymentName,
			"ssh",
			"mysql/0",
			"--results",
			"--command=sudo pmap $(pidof mysqld) | grep jemalloc",
		)
		Expect(err).To(HaveOccurred(),
			`Expected to NOT observe jemalloc in the memory map of the mysqld process`)
	})
})
