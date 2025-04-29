package e2e_tests

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
	"e2e-tests/utilities/credhub"
)

var _ = Describe("Upgrade from pxc v0 to pxc v1", Label("upgrade"), func() {
	It(fmt.Sprintf("can upgrade from pxc-release 5.7 to pxc %s", expectedMysqlVersion), func() {
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
			bosh.VarsEnv("PXC_TEST"),
		)).To(Succeed())

		Expect(bosh.RunErrand(deploymentName, "smoke-tests", "mysql/first")).To(Succeed())

		By("writing data to the 5.7 instance", func() {
			proxyIPs, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("proxy"))
			Expect(err).NotTo(HaveOccurred())
			Expect(proxyIPs).ToNot(BeEmpty())

			db, err := sql.Open("mysql", "test-user:integration-tests@tcp("+proxyIPs[0]+")/?tls=skip-verify")
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			queries := []string{
				`CREATE TABLE upgrade_data.parent (id integer PRIMARY KEY AUTO_INCREMENT, guid varchar(255) NOT NULL)`,
				`CREATE TABLE upgrade_data.child (id integer PRIMARY KEY AUTO_INCREMENT, parent_id integer NOT NULL, CONSTRAINT parent_fk FOREIGN KEY (parent_id) REFERENCES parent(id))`,
				`INSERT INTO upgrade_data.parent VALUES (1, 'ad8cb703-5eb7-4f31-93fa-1f0035e7cab4'), (4, '71229556-0671-4289-9f19-2285590aefbc')`,
			}

			for _, q := range queries {
				Expect(db.Exec(q)).Error().NotTo(HaveOccurred())
			}
		})

		By("intentionally crashing the first node", func() {
			proxyIPs, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("proxy"))
			Expect(err).NotTo(HaveOccurred())
			Expect(proxyIPs).ToNot(BeEmpty())

			db, err := sql.Open("mysql", "test-user:integration-tests@tcp("+proxyIPs[0]+")/?tls=skip-verify")
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			Expect(db.Exec(`CREATE TABLE IF NOT EXISTS upgrade_data.t1 (id int primary key auto_increment, data varchar(40))`)).Error().NotTo(HaveOccurred())

			Consistently(func() error {
				_, err := db.Exec(`INSERT INTO upgrade_data.t1 (data) VALUES (SHA1(RAND()))`)
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

		By(fmt.Sprintf("upgrading single instance 5.7 to clustered PXC %s", expectedMysqlVersion))
		By("Using a collation-server not compatible with 5.7")
		if expectedMysqlVersion == "8.0" {
			By("Using a collation-server not compatible with 5.7")
			Expect(bosh.DeployPXC(deploymentName,
				bosh.Operation("use-clustered.yml"),
				bosh.Operation(`iaas/cluster.yml`),
				bosh.Operation("test/collation-server.yml"),
			)).To(Succeed())
		} else {
			Expect(bosh.DeployPXC(deploymentName,
				bosh.Operation("use-clustered.yml"),
				bosh.Operation(`iaas/cluster.yml`),
			)).To(Succeed())
		}

		Expect(bosh.RunErrand(deploymentName, "smoke-tests", "mysql/first")).To(Succeed())
		if expectedMysqlVersion == "8.0" {
			By("asserting pxc-5.7 actually went through crash recovery", func() {
				output, err := bosh.Logs(deploymentName, "mysql/0", "pxc-mysql/pxc-57-recovery.log")
				Expect(err).NotTo(HaveOccurred())
				Expect(output.String()).To(ContainSubstring(`InnoDB: Starting crash recovery.`))
			})
		}
		By("asserting cluster-health-logger is still able to connect to its local instance", func() {
			output, err := bosh.Logs(deploymentName, "mysql/0", "cluster-health-logger/cluster-health-logger.stderr.log")
			Expect(err).NotTo(HaveOccurred())
			Expect(output.String()).NotTo(ContainSubstring(`Access denied for user 'cluster-health-logger'`))
		})

		By("applying a schema change after the upgrade", func() {
			proxyIPs, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("proxy"))
			Expect(err).NotTo(HaveOccurred())
			Expect(proxyIPs).ToNot(BeEmpty())

			db, err := sql.Open("mysql", "test-user:integration-tests@tcp("+proxyIPs[0]+")/?tls=skip-verify")
			Expect(err).NotTo(HaveOccurred())
			defer db.Close()

			Expect(db.Exec(`ALTER TABLE upgrade_data.parent ADD COLUMN c1 bigint NOT NULL DEFAULT -1`)).Error().NotTo(HaveOccurred())

		})

		By("asserting the data has not been corrupted", func() {
			mysqlIPs, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("mysql"))
			Expect(err).NotTo(HaveOccurred())
			Expect(mysqlIPs).ToNot(BeEmpty())

			verifyData := func(host string) {
				GinkgoHelper()

				db, err := sql.Open("mysql", "test-user:integration-tests@tcp("+host+")/?tls=skip-verify")
				Expect(err).NotTo(HaveOccurred())
				defer db.Close()

				var nodeName string
				Expect(db.QueryRow(`SELECT @@global.wsrep_node_name`).Scan(&nodeName)).To(Succeed())

				var values []string
				query := `SELECT guid FROM upgrade_data.parent FORCE INDEX (PRIMARY)`
				rows, err := db.Query(query)
				Expect(err).NotTo(HaveOccurred())
				defer rows.Close()
				for rows.Next() {
					var guid string
					Expect(rows.Scan(&guid)).To(Succeed())
					values = append(values, guid)
				}
				Expect(rows.Err()).To(BeNil())
				Expect(values).
					To(ConsistOf(`ad8cb703-5eb7-4f31-93fa-1f0035e7cab4`, `71229556-0671-4289-9f19-2285590aefbc`),
						`Expected data to match on instance %q, but it did not`, nodeName)
			}

			for _, host := range mysqlIPs {
				verifyData(host)
			}
		})
	})
})
