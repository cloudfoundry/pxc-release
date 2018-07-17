package failover_test

import (
	"fmt"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"

	helpers "specs/test_helpers"
	"strings"
)

func deleteMysqlVM(host string) error {
	director, err := helpers.BuildBoshDirector()
	if err != nil {
		return fmt.Errorf("building director: %s", err)
	}

	deployment, err := director.FindDeployment(helpers.BoshDeployment())
	if err != nil {
		return fmt.Errorf("finding deployment: %s", err)
	}

	instances, err := deployment.Instances()
	if err != nil {
		return fmt.Errorf("listing instances: %s", err)
	}

	var vmcid string
	for _, instance := range instances {
		if instance.Group == "mysql" {
			hostArray := strings.Split(host, ".")
			if instance.IPs[0] == host || (len(hostArray) > 0 && hostArray[0] == instance.ID) {
				vmcid = instance.VMID
				break
			}
		}
	}

	if vmcid == "" {
		return fmt.Errorf("no vm found with %s", host)
	}

	return deployment.DeleteVM(vmcid)
}

var _ = Describe("CF PXC MySQL Failover", func() {

	BeforeEach(func() {
		helpers.DbSetup("failover_test_table")
	})

	AfterEach(func() {
		helpers.DbCleanup()
	})

	It("proxies failover to another node after a partition of mysql node", func() {
		var oldBackend string
		dbConn := helpers.DbConn()
		query := "INSERT INTO failover_test_table VALUES('the only data')"
		_, err := dbConn.Query(query)
		Expect(err).NotTo(HaveOccurred())

		By("querying the proxy for the current mysql backend", func() {
			var err error

			oldBackend, err = helpers.ActiveProxyBackend()
			Expect(err).NotTo(HaveOccurred())
		})

		By("Take down the active mysql node", func() {
			err := deleteMysqlVM(oldBackend)
			Expect(err).NotTo(HaveOccurred())

		})

		By("poll the proxy for a backend change", func() {
			Eventually(func() bool {
				backend, err := helpers.ActiveProxyBackend()
				Expect(err).NotTo(HaveOccurred())

				return backend != oldBackend
			}, 5*time.Minute, 20*time.Second).Should(BeTrue())
		})

		var queryResultString string
		query = "SELECT * FROM failover_test_table"
		rows, err := dbConn.Query(query)
		Expect(err).NotTo(HaveOccurred())

		rows.Next()
		rows.Scan(&queryResultString)

		Expect(queryResultString).To(Equal("the only data"))
	})

})
