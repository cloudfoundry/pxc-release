package e2e_tests

import (
	"database/sql"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
	"e2e-tests/utilities/credhub"
)

var _ = Describe("Upgrade", Label("upgrade"), func() {
	It("can upgrade from pxc-release 5.7 to pxc 8.0", func() {
		deploymentName := "pxc-upgrade-" + uuid.New().String()

		DeferCleanup(func() {
			if CurrentSpecReport().Failed() {
				return
			}
			Expect(bosh.DeleteDeployment(deploymentName)).To(Succeed())
		})

		By("deploying pxc-release 0.x based on PXC 5.7")
		Expect(bosh.Deploy(deploymentName, "manifest/pxc-5.7.yml",
			bosh.Var("deployment_name", deploymentName),
		)).To(Succeed())

		Expect(bosh.RunErrand(deploymentName, "smoke-tests", "mysql/first")).To(Succeed())

		By("intentionally crashing the first node", func() {
			proxyIPs, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("proxy"))
			Expect(err).NotTo(HaveOccurred())
			Expect(proxyIPs).ToNot(BeEmpty())

			db, err := sql.Open("mysql", "test-user:integration-tests@tcp("+proxyIPs[0]+")/?tls=skip-verify")
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

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

		By("upgrading pxc-release based on PXC 8.0 and using a collation-server not compatible with 5.7")
		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation("use-clustered.yml"),
			bosh.Operation("dev-release.yml"),
			bosh.Operation("test/collation-server.yml"),
		)).To(Succeed())

		Expect(bosh.RunErrand(deploymentName, "smoke-tests", "mysql/first")).To(Succeed())

		By("asserting pxc-5.7 actually went through crash recovery", func() {
			output, err := bosh.Logs(deploymentName, "mysql/0", "pxc-mysql/pxc-57-recovery.log")
			Expect(err).NotTo(HaveOccurred())
			Expect(output.String()).To(ContainSubstring(`InnoDB: Starting crash recovery.`))
		})

		By("asserting cluster-health-logger is still able to connect to its local instance", func() {
			output, err := bosh.Logs(deploymentName, "mysql/0", "cluster-health-logger/cluster-health-logger.stderr.log")
			Expect(err).NotTo(HaveOccurred())
			Expect(output.String()).NotTo(ContainSubstring(`Access denied for user 'cluster-health-logger'`))
		})
	})
})
