package e2e_tests

import (
	"database/sql"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
)

var _ = Describe("Version Deploy", Ordered, Label("version-deploy"), func() {
	var (
		db             *sql.DB
		deploymentName string
	)

	BeforeAll(func() {
		deploymentName = "pxc-single-node-version" + uuid.New().String()

		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation(`with-pxc-57.yml`),
			bosh.Operation(`test/seed-test-user.yml`), // this should go away
		)).To(Succeed())

		Expect(bosh.RunErrand(deploymentName, "smoke-tests", "mysql/first")).To(Succeed())

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

	It("runs Percona XtraDB Cluster v5.7.x", func() {
		var mysqlVersion string
		Expect(db.QueryRow(`SELECT @@global.version`).Scan(&mysqlVersion)).To(Succeed())
		Expect(mysqlVersion).To(HavePrefix("5.7."))
	})

	It("does not set user attributes for smoke tests user", func() {
		var value string
		err := db.QueryRow(`SELECT user FROM INFORMATION_SCHEMA.USER_ATTRIBUTES where user = 'smoke-tests-user' and attribute is null limit 1;`).Scan(&value)
		Expect(err).NotTo(HaveOccurred())
		Expect(value).To(Equal("smoke-tests-user"))
	})
})
