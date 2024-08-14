package e2e_tests

import (
	"database/sql"
	"strings"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
	"e2e-tests/utilities/cmd"
)

var _ = Describe("Scaling", Ordered, Label("scaling"), func() {
	var (
		db             *sql.DB
		deploymentName string
	)

	BeforeAll(func() {
		deploymentName = "pxc-scaling-" + uuid.New().String()

		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation("use-clustered.yml"),
			bosh.Operation("disable-binlog.yml"),
			bosh.Operation("test/seed-test-user.yml"),
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

	Context("OS configuration", Label("os_config"), func() {
		It("uses the default vm.swappiness when no special configuraiton is provided", Label("os_config"), func() {
			swappinessValues, err := bosh.RemoteCommand(deploymentName, "mysql", "cat /proc/sys/vm/swappiness")
			Expect(err).NotTo(HaveOccurred())

			Expect(strings.Fields(swappinessValues)).To(ConsistOf("60", "60", "60"),
				`Expected vm.swappiness to be 1 on all mysql nodes, but it was not!`)
		})
	})

	It("disables the binary log given the disable-binlog ops-file", func() {
		_, err := db.Exec(`SHOW BINARY LOGS`)
		Expect(err).To(MatchError(ContainSubstring(`You are not using binary logging`)))
	})

	It("can write data to this healthy database", func() {
		Expect(db.Exec(`CREATE DATABASE IF NOT EXISTS pxc_release_test_db`)).
			Error().NotTo(HaveOccurred())
		Expect(db.Exec(`CREATE TABLE IF NOT EXISTS pxc_release_test_db.scaling_test (test_data varchar(255) PRIMARY KEY)`)).
			Error().NotTo(HaveOccurred())
		Expect(db.Exec(`INSERT INTO pxc_release_test_db.scaling_test VALUES('data written with 3 nodes')`)).
			Error().NotTo(HaveOccurred())
	})

	It("scales the cluster down to one node", func() {
		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation("use-clustered.yml"),
			bosh.Operation("minimal-mode.yml"),
			bosh.Operation("test/seed-test-user.yml"),
		)).To(Succeed())

		var unused, clusterSize string
		Expect(db.QueryRow(`SHOW GLOBAL STATUS LIKE 'wsrep\_cluster\_size'`).
			Scan(&unused, &clusterSize)).To(Succeed())
		Expect(clusterSize).To(Equal("1"))
	})

	It("can write data to this scaled-down database", func() {
		Expect(db.Exec(`INSERT INTO pxc_release_test_db.scaling_test VALUES('data written with 1 nodes')`)).
			Error().NotTo(HaveOccurred())
	})

	verifyData := func(db *sql.DB) {
		var result []string
		rows, err := db.Query(`SELECT test_data FROM pxc_release_test_db.scaling_test`)
		Expect(err).NotTo(HaveOccurred())
		defer rows.Close()

		for rows.Next() {
			var data string
			Expect(rows.Scan(&data)).To(Succeed())
			result = append(result, data)
		}
		Expect(rows.Err()).NotTo(HaveOccurred())

		Expect(result).To(ConsistOf(
			"data written with 1 nodes",
			"data written with 3 nodes",
		))
	}

	It("retains the data from three nodes still", func() {
		verifyData(db)
	})

	It("can scale back up to three nodes", func() {
		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation("use-clustered.yml"),
			bosh.Operation("test/seed-test-user.yml"),
		)).To(Succeed())
	})

	It("retains the data on every mysql vm", func() {
		mysqlIps, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("mysql"))
		Expect(err).NotTo(HaveOccurred())
		Expect(mysqlIps).To(HaveLen(3))

		for _, host := range mysqlIps {
			db, err := sql.Open("mysql", "test-admin:integration-tests@tcp("+host+")/pxc_release_test_db?tls=skip-verify")
			Expect(err).NotTo(HaveOccurred())
			verifyData(db)
		}
	})

	It("still can access the data through the proxy ip", func() {
		verifyData(db)
	})

	It("rejects scaling down when there is an unhealthy cluster member", func() {
		Expect(bosh.RemoteCommand(deploymentName, "mysql/2",
			"sudo monit unmonitor galera-init && sudo /var/vcap/jobs/bpm/bin/bpm stop pxc-mysql -p galera-init")).
			Error().NotTo(HaveOccurred())

		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation("use-clustered.yml"),
			bosh.Operation("test/seed-test-user.yml"),
			bosh.Operation("minimal-mode.yml"),
		)).ToNot(Succeed())
	})

	It("still allows deleting an unhealthy cluster", func() {
		// Disable normal galera-agent drain to reliably detect failures, by forcing the mysql drain script to run completely
		Expect(bosh.RemoteCommand(deploymentName, "mysql/2",
			"sudo rm -f /var/vcap/jobs/galera-agent/bin/drain")).
			Error().NotTo(HaveOccurred())

		// Skip delete-deployment --force option, as the "force" option ignores drain failures
		// This better emulates many workflows like on-demand-service-broker that fail on drain failures
		Expect(cmd.Run(
			"bosh",
			"--deployment="+deploymentName,
			"--non-interactive",
			"delete-deployment",
		)).To(Succeed())
	})
})
