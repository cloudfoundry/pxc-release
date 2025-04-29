package e2e_tests

import (
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"e2e-tests/utilities/bosh"
)

// Test that injecting additional jobs w/ a bosh wait script does the right thing
var _ = Describe("Wait scripts", Ordered, Label("bosh-wait-scripts"), func() {
	var (
		deploymentName string
		db             *sql.DB
		proxyHost      string
	)

	BeforeAll(func() {
		deploymentName = "pxc-bosh-wait-scripts-" + uuid.New().String()

		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation(`use-clustered.yml`),
			bosh.Operation(`test/seed-test-user.yml`),
			bosh.Operation(`iaas/cluster.yml`),
		)).To(Succeed())

		Expect(bosh.RunErrand(deploymentName, "smoke-tests", "mysql/first")).
			To(Succeed())

		proxyIPs, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("proxy"))
		Expect(err).NotTo(HaveOccurred())
		Expect(proxyIPs).To(HaveLen(2))
		proxyHost = proxyIPs[0]

		db, err = sql.Open("mysql", "test-admin:integration-tests@tcp("+proxyHost+")/?tls=skip-verify&interpolateParams=true")
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

	copyWaitScript := func(name string) {
		Expect(
			bosh.RemoteCommand(
				deploymentName,
				"mysql",
				"sudo mkdir -p /var/vcap/jobs/"+name+"/bin/disks/"),
		).Error().NotTo(HaveOccurred())

		Expect(
			bosh.Scp(
				deploymentName,
				"fixtures/"+name,
				"mysql:/tmp/"+name,
			),
		).Error().NotTo(HaveOccurred())

		Expect(
			bosh.RemoteCommand(
				deploymentName,
				"mysql",
				fmt.Sprintf("sudo mv /tmp/%s /var/vcap/jobs/%s/bin/disks/wait", name, name),
			),
		).Error().NotTo(HaveOccurred())

		Expect(
			bosh.RemoteCommand(
				deploymentName,
				"mysql",
				fmt.Sprintf("sudo chmod +x /var/vcap/jobs/%s/bin/disks/wait", name),
			),
		).Error().NotTo(HaveOccurred())
	}

	When("a BOSH job provides disk wait scripts that run successfully", func() {
		BeforeEach(func() {
			copyWaitScript("successful_wait_script_1")
			copyWaitScript("successful_wait_script_2")

			Expect(
				bosh.Restart(deploymentName, "mysql"),
			).To(Succeed())
		})

		AfterEach(func() {
			Expect(
				bosh.RemoteCommand(deploymentName, "mysql", `sudo rm -rf /var/vcap/jobs/successful_wait_script`),
			).Error().NotTo(HaveOccurred())
		})

		It("waits until all the wait scripts have completed successfully to start mysql", func() {
			var (
				maxErrorCount    int
				maxConnectErrors int
			)
			Expect(db.QueryRow(`SELECT @@global.max_error_count, @@global.max_connect_errors`).
				Scan(&maxErrorCount, &maxConnectErrors)).To(Succeed())

			Expect(maxErrorCount).To(Equal(123))
			Expect(maxConnectErrors).To(Equal(456))
		})
	})

	When("BOSH jobs provide disk wait scripts fails", func() {
		BeforeEach(func() {
			copyWaitScript("failed_wait_script")
		})

		AfterEach(func() {
			Expect(
				bosh.RemoteCommand(deploymentName, "mysql", `sudo rm -rf /var/vcap/jobs/failed_wait_script*`),
			).Error().NotTo(HaveOccurred())
		})

		It("fails the deployment", func() {
			Expect(bosh.Restart(deploymentName, "mysql")).
				NotTo(Succeed())
		})
	})
})
