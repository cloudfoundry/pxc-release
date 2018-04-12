package failover_test

import (
	"database/sql"
	"encoding/json"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"net/http"
	"os"
	"time"

	helpers "specs/test_helpers"
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
		if instance.Group == "mysql" && instance.IPs[0] == host {
			vmcid = instance.VMID
			break
		}
	}

	if vmcid == "" {
		return fmt.Errorf("no vm found with %s", host)
	}

	return deployment.DeleteVM(vmcid)
}

func activeProxyBackend() (string, error) {
	client := &http.Client{}

	var proxyUsername = os.Getenv("PROXY_USERNAME")
	var proxyPassword = os.Getenv("PROXY_PASSWORD")

	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s:8080/v0/cluster", helpers.BoshEnvironment()), nil)
	if err != nil {
		return "", err
	}

	req.SetBasicAuth(proxyUsername, proxyPassword)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("ERROR: Non-200 received from proxy. Status: %s", resp.Status)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var cluster struct {
		ActiveBackend struct {
			Host string `json:"host"`
		} `json:"activeBackend`
	}

	if err := json.Unmarshal(body, &cluster); err != nil {
		return "", err
	}

	return cluster.ActiveBackend.Host, nil
}

var _ = Describe("CF PXC MySQL Failover", func() {
	var (
		pxcConnectionString string
	)

	BeforeEach(func() {
		var mysqlUsername = os.Getenv("MYSQL_USERNAME")
		var mysqlPassword = os.Getenv("MYSQL_PASSWORD")

		pxcConnectionString = fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/",
			mysqlUsername,
			mysqlPassword,
			helpers.BoshEnvironment(),
			3306)
		databaseConnection, err := sql.Open("mysql", pxcConnectionString)
		Expect(err).NotTo(HaveOccurred())

		statement := "CREATE DATABASE IF NOT EXISTS failover_test1"
		_, err = databaseConnection.Exec(statement)
		Expect(err).NotTo(HaveOccurred())

		pxcConnectionString = fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/failover_test1",
			mysqlUsername,
			mysqlPassword,
			helpers.BoshEnvironment(),
			3306)

		statement = "USE failover_test1"
		_, err = databaseConnection.Exec(statement)
		Expect(err).NotTo(HaveOccurred())

		statement = "CREATE TABLE IF NOT EXISTS failover_test_table (id varchar(255))"
		_, err = databaseConnection.Exec(statement)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		databaseConnection, err := sql.Open("mysql", pxcConnectionString)
		Expect(err).NotTo(HaveOccurred())

		statement := "DROP DATABASE failover_test1"
		_, err = databaseConnection.Exec(statement)
		Expect(err).NotTo(HaveOccurred())
	})

	It("proxies failover to another node after a partition of mysql node", func() {
		var oldBackend string

		databaseConnection, err := sql.Open("mysql", pxcConnectionString)
		Expect(err).NotTo(HaveOccurred())

		query := "INSERT INTO failover_test_table VALUES('the only data')"
		_, err = databaseConnection.Query(query)
		Expect(err).NotTo(HaveOccurred())

		By("querying the proxy for the current mysql backend", func() {
			var err error

			oldBackend, err = activeProxyBackend()
			Expect(err).NotTo(HaveOccurred())
		})

		By("Take down the active mysql node", func() {
			err := deleteMysqlVM(oldBackend)
			Expect(err).NotTo(HaveOccurred())

		})

		By("poll the proxy for a backend change", func() {
			Eventually(func() bool {
				backend, err := activeProxyBackend()
				Expect(err).NotTo(HaveOccurred())

				return backend != oldBackend
			}, 5*time.Minute, 20*time.Second).Should(BeTrue())
		})

		var queryResultString string
		query = "SELECT * FROM failover_test_table"
		rows, err := databaseConnection.Query(query)
		Expect(err).NotTo(HaveOccurred())

		rows.Next()
		rows.Scan(&queryResultString)

		Expect(queryResultString).To(Equal("the only data"))
	})

})
