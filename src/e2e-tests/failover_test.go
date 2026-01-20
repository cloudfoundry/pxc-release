package e2e_tests

import (
	"database/sql"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
	"e2e-tests/utilities/cmd"
)

var _ = Describe("Failover", Ordered, Label("failover"), func() {
	var (
		db             *sql.DB
		deploymentName string
	)

	BeforeAll(func() {
		deploymentName = "pxc-failover-" + uuid.New().String()

		Expect(cmd.Run(
			"bosh", "update-resurrection", "-d", deploymentName, "off",
		)).To(Succeed())

		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation("use-clustered.yml"),
			bosh.Operation("test/seed-test-user.yml"),
			bosh.Operation(`iaas/cluster.yml`),
		)).To(Succeed())

		Expect(bosh.RunErrand(deploymentName, "smoke-tests", "mysql/first")).
			To(Succeed())

		proxyIPs, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("proxy"))
		Expect(err).NotTo(HaveOccurred())
		Expect(proxyIPs).To(HaveLen(2))

		db, err = sql.Open("mysql", "test-admin:integration-tests@tcp("+proxyIPs[0]+")/?tls=skip-verify")
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

	It("starts with a healthy cluster of three nodes", func() {
		var unused, clusterSize string
		Expect(db.QueryRow(`SHOW GLOBAL STATUS LIKE 'wsrep\_cluster\_size'`).
			Scan(&unused, &clusterSize)).To(Succeed())
		Expect(clusterSize).To(Equal("3"))
	})

	It("can write data to this healthy database", func() {
		Expect(db.Exec(`CREATE DATABASE IF NOT EXISTS pxc_release_test_db`)).
			Error().NotTo(HaveOccurred())
		Expect(db.Exec(`CREATE TABLE IF NOT EXISTS pxc_release_test_db.failover_test (test_data varchar(255) PRIMARY KEY)`)).
			Error().NotTo(HaveOccurred())
		Expect(db.Exec(`INSERT INTO pxc_release_test_db.failover_test VALUES(CONCAT(?, ': data written with 3 nodes'))`, deploymentName)).
			Error().NotTo(HaveOccurred())
	})

	When("the active member of the cluster fails", Ordered, func() {
		var (
			originalBackend string
		)
		BeforeAll(func() {
			var activeNodeName string
			Expect(db.QueryRow(`SELECT @@global.wsrep_node_name`).
				Scan(&activeNodeName)).To(Succeed())

			originalBackend = activeNodeName

			instances, err := bosh.Instances(deploymentName, bosh.MatchByIndexedName(activeNodeName))
			Expect(err).NotTo(HaveOccurred())
			Expect(instances).To(HaveLen(1))

			Expect(bosh.DeleteVM(deploymentName, instances[0].VMCid)).To(Succeed())
		})

		It("eventually observes a two node cluster", func() {
			Eventually(func() string {
				var unused, clusterSize string
				_ = db.QueryRow(`SHOW GLOBAL STATUS LIKE 'wsrep\_cluster\_size'`).
					Scan(&unused, &clusterSize)
				return clusterSize
			}, "5m", "5s").Should(Equal("2"))
		})

		It("observes the proxy forwards to a new backend", func() {
			var activeNodeName string
			Expect(db.QueryRow(`SELECT @@global.wsrep_node_name`).
				Scan(&activeNodeName)).To(Succeed())
			Expect(activeNodeName).ToNot(Equal(originalBackend))
		})

		It("observes data retained in the surviving cluster members", func() {
			var data string
			Expect(db.QueryRow(`SELECT test_data FROM pxc_release_test_db.failover_test`).
				Scan(&data)).To(Succeed())

			Expect(data).To(Equal(deploymentName + ": data written with 3 nodes"))
		})

		When("the failed VM is restored", func() {
			BeforeAll(func() {
				_ = bosh.CloudCheck(deploymentName, "--resolution=recreate_vm")
				Expect(bosh.CloudCheck(deploymentName, "--report")).To(Succeed())
			})

			It("observes the cluster eventually fully recovers", func() {
				Eventually(func() string {
					var unused, clusterSize string
					_ = db.QueryRow(`SHOW GLOBAL STATUS LIKE 'wsrep\_cluster\_size'`).
						Scan(&unused, &clusterSize)
					return clusterSize
				}, "10m", "1s").Should(Equal("3"))
			})

			It("observes the cluster still retains the expected data", func() {
				mysqlIps, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("mysql"))
				Expect(err).NotTo(HaveOccurred())

				Expect(mysqlIps).To(HaveLen(3))

				for _, host := range mysqlIps {
					db, err := sql.Open("mysql", "test-admin:integration-tests@tcp("+host+")/pxc_release_test_db?tls=skip-verify")
					Expect(err).NotTo(HaveOccurred())
					var data string
					Expect(db.QueryRow(`SELECT test_data FROM pxc_release_test_db.failover_test`).
						Scan(&data)).To(Succeed())
					Expect(data).To(Equal(deploymentName + ": data written with 3 nodes"))
				}
			})
		})
	})
})
