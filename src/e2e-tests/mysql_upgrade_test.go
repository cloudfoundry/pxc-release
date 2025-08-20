package e2e_tests

import (
	"database/sql"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
	"e2e-tests/utilities/credhub"
)

var _ = Describe("MySQL Version Upgrades in pxc v1", Label("mysql-version-upgrade"), Ordered, func() {
	var previousVersion string
	if expectedMysqlVersion == "8.4" {
		previousVersion = "8.0"
	}

	BeforeAll(func() {
		if previousVersion == "" {
			Skip("Current MYSQL_VERSION(" + expectedMysqlVersion + ") has no supported previous version to upgrade from.")
		}
	})

	It("can upgrade from mysql_version="+previousVersion+" to mysql_version="+expectedMysqlVersion, func() {
		deploymentName := "pxc-mysql-version-upgrade-" + uuid.New().String()

		DeferCleanup(func() {
			if CurrentSpecReport().Failed() {
				return
			}
			Expect(bosh.DeleteDeployment(deploymentName)).To(Succeed())
		})

		By("deploying the current pxc-release with mysql_version=" + previousVersion)
		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation("use-clustered.yml"),
			bosh.Operation(`iaas/cluster.yml`),
			bosh.Operation("test/seed-test-user.yml"),
			bosh.Operation(`mysql-version.yml`),
			bosh.Var("mysql_version", previousVersion),
		)).To(Succeed())

		Expect(bosh.RunErrand(deploymentName, "smoke-tests", "mysql/first")).To(Succeed())

		proxyIPs, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("proxy"))
		Expect(err).NotTo(HaveOccurred())
		Expect(proxyIPs).ToNot(BeEmpty())

		db, err := sql.Open("mysql", "test-admin:integration-tests@tcp("+proxyIPs[0]+")/?tls=skip-verify")
		Expect(err).NotTo(HaveOccurred())
		defer db.Close()

		By("asserting the deployed MySQL version was "+previousVersion, func() {
			var mysqlVersion string
			Expect(db.QueryRow(`SELECT @@global.version`).Scan(&mysqlVersion)).To(Succeed())
			Expect(mysqlVersion).To(HavePrefix(previousVersion + "."))
		})

		By("having the test intentionally crashing the first node so that upgrades must perform crash recovery", func() {
			Expect(db.Exec(`CREATE DATABASE IF NOT EXISTS crash_upgrade_test`)).Error().NotTo(HaveOccurred())
			Expect(db.Exec(`CREATE TABLE IF NOT EXISTS crash_upgrade_test.t1 (id int primary key auto_increment, data varchar(40))`)).Error().NotTo(HaveOccurred())

			Consistently(func() error {
				_, err := db.Exec(`INSERT INTO crash_upgrade_test.t1 (data) VALUES (SHA1(RAND()))`)
				return err
			}, "15s").Should(Succeed())

			Expect(bosh.RemoteCommand(deploymentName,
				"mysql/0",
				`sudo monit unmonitor galera-init && sudo killall -9 mysqld`),
			).Error().NotTo(HaveOccurred())
		})

		By("forcing credhub to rotate the galera-agent database credential")
		Expect(credhub.Regenerate("/" + deploymentName + "/cf_mysql_mysql_galera_healthcheck_db_password")).
			To(Succeed())

		By("forcing credhub to rotate the cluster-health-logger database credential")
		Expect(credhub.Regenerate("/" + deploymentName + "/cf_mysql_mysql_cluster_health_password")).
			To(Succeed())

		By("upgrading from mysql_version=" + previousVersion + " to mysql_version=" + expectedMysqlVersion)
		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation("use-clustered.yml"),
			bosh.Operation(`iaas/cluster.yml`),
			bosh.Operation("test/seed-test-user.yml"),
		)).To(Succeed())

		Eventually(db.Ping).Should(Succeed())
		By("asserting the deployed MySQL version was "+expectedMysqlVersion, func() {
			var mysqlVersion string
			Expect(db.QueryRow(`SELECT @@global.version`).Scan(&mysqlVersion)).To(Succeed())
			Expect(mysqlVersion).To(HavePrefix(expectedMysqlVersion))
		})

		By("verify the deployment is working as expected")
		Expect(bosh.RunErrand(deploymentName, "smoke-tests", "mysql/first")).To(Succeed())

		By("asserting mysql actually went performed crash recovery as part of the upgrade process", func() {
			output, err := bosh.Logs(deploymentName, "mysql/0", "pxc-mysql/pxc-*-recovery.log")
			Expect(err).NotTo(HaveOccurred())
			Expect(output.String()).To(MatchRegexp(`Starting .*crash recovery`))
		})

		By("asserting gtid_mode has not been enabled on a cluster by default", func() {
			var gtidExecuted string
			Expect(db.QueryRow(`SELECT @@global.gtid_executed`).Scan(&gtidExecuted)).To(Succeed())
			Expect(gtidExecuted).To(BeEmpty())
		})
	})
})
