package bootstrap_test

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"time"

	boshdir "github.com/cloudfoundry/bosh-cli/director"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"

	helpers "specs/test_helpers"
)

func stopMySQL(host string) error {
	stopMySQLEndpoint := fmt.Sprintf("http://%s:9200/stop_mysql", host)
	req, err := http.NewRequest("POST", stopMySQLEndpoint, nil)
	if err != nil {
		return err
	}

	galeraAgentPassword, err := helpers.GetGaleraAgentPassword()
	if err != nil {
		return err
	}

	req.SetBasicAuth(galeraAgentUsername, galeraAgentPassword)

	res, err := helpers.HttpClient.Do(req)
	if err != nil {
		return err
	}

	responseBody, _ := ioutil.ReadAll(res.Body)
	fmt.Fprintln(GinkgoWriter, string(responseBody))

	if res.StatusCode != http.StatusOK {
		return errors.Errorf(`Expected [HTTP 200], but got %s. body: %v`, res.Status, string(responseBody))
	}

	return nil
}

func stopGaleraInitOnAllMysqls() {
	mysqlHosts, err := helpers.MySQLHosts(helpers.BoshDeployment)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	for _, host := range mysqlHosts {
		ExpectWithOffset(1, stopMySQL(host)).To(Succeed())
	}

	firstProxy, err := helpers.FirstProxyHost(helpers.BoshDeployment)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	proxyPassword, err := helpers.GetProxyPassword()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	EventuallyWithOffset(1, func() (string, error) {
		return helpers.ActiveProxyBackend(proxyUsername, proxyPassword, firstProxy, helpers.HttpClient)
	}, "3m", "1s").Should(BeEmpty())
}

func bootstrapCluster() {
	slugList := []boshdir.InstanceGroupOrInstanceSlug{boshdir.NewInstanceGroupOrInstanceSlug("mysql", "0")}
	errandResult, err := helpers.BoshDeployment.RunErrand("bootstrap", false, false, slugList)
	Expect(err).NotTo(HaveOccurred())

	fmt.Println(fmt.Sprintf("Errand STDOUT: %s", errandResult[0].Stdout))
	fmt.Println(fmt.Sprintf("Errand STDERR: %s", errandResult[0].Stderr))
}

var _ = Describe("CF PXC MySQL Bootstrap", func() {
	BeforeEach(func() {
		helpers.DbSetup(mysqlConn, "bootstrap_test_table")
	})

	AfterEach(func() {
		helpers.DbCleanup(mysqlConn)
	})

	It("bootstraps a cluster", func() {
		By("Write data")
		_, err := mysqlConn.Query("INSERT INTO pxc_release_test_db.bootstrap_test_table VALUES('the only data')")
		Expect(err).NotTo(HaveOccurred())

		By("Stop all instances of mysql")
		stopGaleraInitOnAllMysqls()

		By("Wait for monit to finish stopping")
		time.Sleep(5 * time.Second)

		bootstrapCluster()

		By("Verify cluster has three nodes")
		var variableName, variableValue string
		rows, err := mysqlConn.Query("SHOW status LIKE 'wsrep_cluster_size'")
		Expect(err).NotTo(HaveOccurred())

		Expect(rows.Next()).To(BeTrue())
		Expect(rows.Scan(&variableName, &variableValue)).To(Succeed())

		Expect(variableValue).To(Equal("3"))

		By("Verifying the data still exists")
		var queryResultString string
		rows, err = mysqlConn.Query("SELECT * FROM pxc_release_test_db.bootstrap_test_table")
		Expect(err).NotTo(HaveOccurred())

		Expect(rows.Next()).To(BeTrue())
		Expect(rows.Scan(&queryResultString)).To(Succeed())
		Expect(queryResultString).To(Equal("the only data"))
	})
})
