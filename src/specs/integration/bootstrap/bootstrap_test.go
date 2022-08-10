package bootstrap_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os/exec"
	"strings"
	"time"

	boshdir "github.com/cloudfoundry/bosh-cli/v7/director"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"

	helpers "github.com/cloudfoundry/pxc-release/specs/test_helpers"
)

func doPost(url string, galeraAgentPassword string) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(galeraAgentUsername, galeraAgentPassword)
	return helpers.HttpClient.Do(req)
}

func stopMySQL(host string) error {
	galeraAgentPassword, err := helpers.GetGaleraAgentPassword()
	if err != nil {
		return err
	}

	stopMySQLEndpoint := fmt.Sprintf("http://%s:9200/stop_mysql", host)
	isEnabled, err := helpers.IsTLSEnabled("/instance_groups/name=mysql/jobs/name=galera-agent/properties/endpoint_tls?/enabled")
	if err != nil {
		return err
	}
	if isEnabled {
		stopMySQLEndpoint = fmt.Sprintf("https://%s:9201/stop_mysql", host)
	}

	res, err := doPost(stopMySQLEndpoint, galeraAgentPassword)

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
		return helpers.ActiveProxyBackend(proxyUsername, proxyPassword, firstProxy)
	}, "3m", "1s").Should(BeEmpty())
}

func bootstrapCluster() {
	slugList := []boshdir.InstanceGroupOrInstanceSlug{boshdir.NewInstanceGroupOrInstanceSlug("mysql", "0")}
	errandResult, err := helpers.BoshDeployment.RunErrand("bootstrap", false, false, slugList)
	Expect(err).NotTo(HaveOccurred())

	fmt.Printf("Errand STDOUT: %s", errandResult[0].Stdout)
	fmt.Printf("Errand STDERR: %s", errandResult[0].Stderr)
}

var _ = Describe("CF PXC MySQL Bootstrap", func() {
	When("manually bootstrapping", func() {
		BeforeEach(func() {
			stopGaleraInitOnAllMysqls()
		})

		AfterEach(func() {
			bootstrapCluster()
		})

		fetchGraStateDat := func() map[string]interface{} {
			cmd := exec.Command("bosh", "ssh", "mysql/0",
				"--results",
				"--column=Stdout",
				"--command",
				"sudo cat /var/vcap/store/pxc-mysql/grastate.dat",
			)
			var buf bytes.Buffer
			cmd.Stdout = io.MultiWriter(GinkgoWriter, &buf)
			cmd.Stderr = GinkgoWriter
			Expect(cmd.Run()).To(Succeed())

			var m map[string]interface{}
			in := bytes.ReplaceAll(buf.Bytes(), []byte("\t"), nil)
			Expect(yaml.Unmarshal(in, &m)).To(Succeed())

			return m
		}

		fetchInstanceID := func() string {
			cmd := exec.Command("bosh", "ssh", "mysql/0",
				"--results",
				"--column=Stdout",
				"--command",
				"cat /var/vcap/instance/id",
			)
			var buf bytes.Buffer
			cmd.Stdout = io.MultiWriter(GinkgoWriter, &buf)
			cmd.Stderr = GinkgoWriter
			Expect(cmd.Run()).To(Succeed())

			return strings.TrimSpace(buf.String())
		}

		It("provides a script to output the galera seqno", func() {
			cmd := exec.Command("bosh", "ssh", "mysql/0",
				"--results",
				"--column=Stdout",
				"--command",
				"sudo /var/vcap/jobs/pxc-mysql/bin/get-sequence-number",
			)
			var buf bytes.Buffer
			cmd.Stdout = io.MultiWriter(GinkgoWriter, &buf)
			cmd.Stderr = GinkgoWriter
			Expect(cmd.Run()).To(Succeed())

			type getSeqNoResponse struct {
				ClusterUUID string `json:"cluster_uuid"`
				Seqno       int    `json:"seqno"`
				InstanceID  string `json:"instance_id"`
			}

			var m getSeqNoResponse
			Expect(json.Unmarshal(buf.Bytes(), &m)).To(Succeed())

			graStateInfo := fetchGraStateDat()
			Expect(m).To(Equal(getSeqNoResponse{
				ClusterUUID: graStateInfo["uuid"].(string),
				Seqno:       graStateInfo["seqno"].(int),
				InstanceID:  fetchInstanceID(),
			}))
		})
	})

	When("running the bootstrap errand", func() {
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
})
