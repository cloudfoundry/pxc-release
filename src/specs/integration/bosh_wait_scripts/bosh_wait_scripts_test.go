package bosh_wait_scripts_test

import (
	"fmt"
	"os"
	"strings"
	"time"

	helpers "specs/test_helpers"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func copyWaitScript(name string, boshDeployment string) {
	dir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	Expect(
		helpers.ExecuteBosh([]string{
			"-d", boshDeployment,
			"ssh",
			"mysql",
			"-c",
			fmt.Sprintf("sudo mkdir -p /var/vcap/jobs/%s/bin/disks/", name),
		}, 2*time.Minute),
	).To(gexec.Exit(0))

	Expect(
		helpers.ExecuteBosh([]string{
			"-d", boshDeployment,
			"scp",
			fmt.Sprintf("%s/fixtures/%s", dir, name),
			fmt.Sprintf("mysql:/tmp/%s", name),
		}, 2*time.Minute),
	).To(gexec.Exit(0))

	Expect(
		helpers.ExecuteBosh([]string{
			"-d", boshDeployment,
			"ssh",
			"mysql",
			"-c",
			fmt.Sprintf("sudo mv /tmp/%s /var/vcap/jobs/%s/bin/disks/wait", name, name),
		}, 2*time.Minute),
	).To(gexec.Exit(0))

	Expect(
		helpers.ExecuteBosh([]string{
			"-d", boshDeployment,
			"ssh",
			"mysql",
			"-c",
			fmt.Sprintf("sudo chmod +x /var/vcap/jobs/%s/bin/disks/wait", name),
		}, 2*time.Minute),
	).To(gexec.Exit(0))
}

var boshDeployment = os.Getenv("BOSH_DEPLOYMENT")

var _ = Describe("PXC Engine", func() {

	Context("when BOSH jobs provide disk wait scripts runs successfully", func() {
		BeforeEach(func() {
			copyWaitScript("successful_wait_script_1", boshDeployment)
			copyWaitScript("successful_wait_script_2", boshDeployment)

			Expect(
				helpers.ExecuteBosh([]string{
					"-d", boshDeployment,
					"-n",
					"restart", "mysql",
				}, 10*time.Minute),
			).To(gexec.Exit(0))
		})

		AfterEach(func() {
			Expect(
				helpers.ExecuteBosh([]string{
					"-d", boshDeployment,
					"ssh",
					"-c",
					"sudo rm -rf /var/vcap/jobs/successful_wait_script*",
				}, 2*time.Minute),
			).To(gexec.Exit(0))
		})

		It("waits until all the wait scripts have completed successfully to start mysql", func() {
			result := helpers.ExecuteMysqlQueryAsAdmin(boshDeployment, "0", "SHOW VARIABLES LIKE 'max_error_count';")
			fields := strings.Fields(result)
			Expect(fields[0]).To(Equal("max_error_count"))
			Expect(fields[1]).To(Equal("123"))

			result = helpers.ExecuteMysqlQueryAsAdmin(boshDeployment, "0", "SHOW VARIABLES LIKE 'max_connect_errors';")
			fields = strings.Fields(result)
			Expect(fields[0]).To(Equal("max_connect_errors"))
			Expect(fields[1]).To(Equal("456"))
		})
	})

	Context("when BOSH jobs provide disk wait scripts fails", func() {
		BeforeEach(func() {
			copyWaitScript("failed_wait_script", boshDeployment)
		})

		AfterEach(func() {
			Expect(
				helpers.ExecuteBosh([]string{
					"-d", boshDeployment,
					"ssh",
					"-c",
					"sudo rm -rf /var/vcap/jobs/failed_wait_script*",
				}, 2*time.Minute),
			).To(gexec.Exit(0))
			Expect(
				helpers.ExecuteBosh([]string{
					"-d", boshDeployment,
					"-n",
					"restart", "mysql",
				}, 10*time.Minute),
			).To(gexec.Exit(0))
		})

		It("fails the deployment", func() {
			Expect(
				helpers.ExecuteBosh([]string{
					"-d", boshDeployment,
					"-n",
					"restart", "mysql",
				}, 10*time.Minute),
			).To(gexec.Exit(1))
		})
	})
})
