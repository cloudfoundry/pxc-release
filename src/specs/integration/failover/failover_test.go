package failover_test

import (
	"database/sql"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	helpers "specs/test_helpers"
	"strings"
)

func deleteMysqlVM(host string) error {
	director, err := helpers.BuildBoshDirector()
	if err != nil {
		return fmt.Errorf("building director: %s", err)
	}

	deployment, err := director.FindDeployment(helpers.BoshDeploymentName())
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
	var (
		db            *sql.DB
		mysqlUsername = "root"
		proxyUsername = "proxy"

		mysqlPassword string
		proxyPassword string
		firstProxy    string
		err           error
	)

	BeforeEach(func() {
		firstProxy, err = helpers.FirstProxyHost(helpers.BoshDeployment)
		Expect(err).NotTo(HaveOccurred())

		proxyPassword, err = helpers.GetProxyPassword()
		Expect(err).NotTo(HaveOccurred())

		mysqlPassword, err = helpers.GetMySQLAdminPassword()
		Expect(err).NotTo(HaveOccurred())

		db = helpers.DbConnWithUser(mysqlUsername, mysqlPassword, firstProxy)
		helpers.DbSetup(db, "failover_test_table")
	})

	AfterEach(func() {
		helpers.DbCleanup(db)
	})

	It("proxies failover to another node after a partition of mysql node", func() {
		var oldBackend string
		query := "INSERT INTO pxc_release_test_db.failover_test_table VALUES('the only data')"
		_, err := mysqlConn.Query(query)
		Expect(err).NotTo(HaveOccurred())

		By("querying the proxy for the current mysql backend", func() {
			var err error
			oldBackend, err = helpers.ActiveProxyBackend(proxyUsername, proxyPassword, firstProxy, helpers.HttpClient)
			Expect(err).NotTo(HaveOccurred())
		})

		By("Take down the active mysql node", func() {
			err := deleteMysqlVM(oldBackend)
			Expect(err).NotTo(HaveOccurred())
		})

		By("poll the proxy for a backend change", func() {
			Eventually(func() bool {
				backend, err := helpers.ActiveProxyBackend(proxyUsername, proxyPassword, firstProxy, helpers.HttpClient)
				Expect(err).NotTo(HaveOccurred())

				return backend != oldBackend
			}, 5*time.Minute, 20*time.Second).Should(BeTrue())
		})

		var queryResultString string
		query = "SELECT * FROM pxc_release_test_db.failover_test_table"
		rows, err := mysqlConn.Query(query)
		Expect(err).NotTo(HaveOccurred())

		rows.Next()
		rows.Scan(&queryResultString)

		Expect(queryResultString).To(Equal("the only data"))
	})

})
