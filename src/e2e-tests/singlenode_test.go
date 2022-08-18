package e2e_tests

import (
	"database/sql"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
)

var _ = Describe("Single Node", Ordered, func() {
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

	It("runs Percona XtraDB Cluster v8.0.x", func() {
		var mysqlVersion string
		Expect(db.QueryRow(`SELECT @@global.version`).Scan(&mysqlVersion)).To(Succeed())
		Expect(mysqlVersion).To(HavePrefix("8.0."))
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
})
