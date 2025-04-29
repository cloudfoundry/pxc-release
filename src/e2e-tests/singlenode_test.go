package e2e_tests

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
)

var _ = Describe(fmt.Sprintf("Single Node for PXC version %s", expectedMysqlVersion), Ordered, Label("single-node"), func() {
	var (
		db             *sql.DB
		deploymentName string
	)

	BeforeAll(func() {
		deploymentName = "pxc-single-node-" + uuid.New().String()

		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation(`iaas/single-node.yml`),
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

	It("does not enable jemalloc by default", Label("jemalloc"), func() {
		_, err := bosh.RemoteCommand(deploymentName, "mysql/0", "! sudo grep -q jemalloc /proc/$(pidof mysqld)/maps")
		Expect(err).NotTo(HaveOccurred(),
			`Expected jemalloc to not be enabled by default, but it was present in /proc/$(pidof mysqld)/maps!`)
	})
})
