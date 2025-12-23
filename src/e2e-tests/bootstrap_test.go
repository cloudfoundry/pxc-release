package e2e_tests

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"os/exec"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"gopkg.in/yaml.v3"

	"e2e-tests/utilities/bosh"
	"e2e-tests/utilities/cmd"
	"e2e-tests/utilities/credhub"
)

var _ = Describe("Bootstrapping an offline cluster", Ordered, Label("bootstrap"), func() {
	var (
		db                  *sql.DB
		deploymentName      string
		galeraAgentPassword string
	)

	BeforeAll(func() {
		deploymentName = "pxc-bootstrap-" + uuid.New().String()

		Expect(bosh.DeployPXC(deploymentName,
			bosh.Operation("use-clustered.yml"),
			bosh.Operation("galera-agent-tls.yml"),
			bosh.Operation("test/seed-test-user.yml"),
			bosh.Operation("iaas/cluster.yml"),
		)).To(Succeed())
		DeferCleanup(func() {
			if CurrentSpecReport().Failed() {
				return
			}

			Expect(bosh.DeleteDeployment(deploymentName)).To(Succeed())
		})

		Expect(bosh.RunErrand(deploymentName, "smoke-tests", "mysql/first")).
			To(Succeed())

		proxyIPs, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("proxy"))
		Expect(err).NotTo(HaveOccurred())
		Expect(proxyIPs).To(HaveLen(2))

		db, err = sql.Open("mysql", "test-admin:integration-tests@tcp("+proxyIPs[0]+")/?tls=skip-verify")
		Expect(err).NotTo(HaveOccurred())
		db.SetMaxIdleConns(0)
		db.SetMaxOpenConns(1)

		galeraAgentPassword, err = credhub.GetCredhubPassword("/" + deploymentName + "/cf_mysql_mysql_galera_healthcheck_endpoint_password")
		Expect(err).NotTo(HaveOccurred())
	})

	stopMySQL := func(c *http.Client, host string) {
		req, err := http.NewRequest(http.MethodPost, "https://"+host+":9201/stop_mysql", nil)
		Expect(err).NotTo(HaveOccurred())
		req.SetBasicAuth("galera-agent", galeraAgentPassword)
		res, err := c.Do(req)
		Expect(err).NotTo(HaveOccurred())
		body, _ := io.ReadAll(res.Body)
		Expect(res.StatusCode).To(Equal(http.StatusOK),
			`Expected HTTP 200 from stop_mysql but got %q.  Body: %s`, res.Status, string(body))
	}

	It("starts with a healthy cluster of three nodes", func() {
		var unused, clusterSize string
		Expect(db.QueryRow(`SHOW GLOBAL STATUS LIKE 'wsrep\_cluster\_size'`).
			Scan(&unused, &clusterSize)).To(Succeed())
		Expect(clusterSize).To(Equal("3"))
	})

	It("can write data to this healthy database", func() {
		Expect(db.Exec(`CREATE DATABASE IF NOT EXISTS pxc_release_test_db`)).
			Error().NotTo(HaveOccurred())
		Expect(db.Exec(`CREATE TABLE IF NOT EXISTS pxc_release_test_db.bootstrap_test (test_data varchar(255) PRIMARY KEY)`)).
			Error().NotTo(HaveOccurred())
		Expect(db.Exec(`INSERT INTO pxc_release_test_db.bootstrap_test VALUES(CONCAT(?, ': data written with 3 nodes'))`, deploymentName)).
			Error().NotTo(HaveOccurred())
	})

	When("the entire cluster goes offline", Ordered, func() {
		BeforeAll(func() {
			By("mysql shutting down on all nodes")
			mysqlIps, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("mysql"))
			Expect(err).NotTo(HaveOccurred())

			for _, ip := range mysqlIps {
				stopMySQL(httpClient, ip)
			}
		})

		It("supports manually discovering the most up-to-date node by running the get-sequence-number script", func() {
			output, err := bosh.RemoteCommand(deploymentName, "mysql/0",
				"sudo /var/vcap/jobs/pxc-mysql/bin/get-sequence-number")
			Expect(err).NotTo(HaveOccurred())

			type getSeqNoResponse struct {
				ClusterUUID string `json:"cluster_uuid"`
				Seqno       int    `json:"seqno"`
				InstanceID  string `json:"instance_id"`
			}

			var getSeqNoParsed getSeqNoResponse
			Expect(json.Unmarshal([]byte(output), &getSeqNoParsed)).To(Succeed())

			graStateRaw, err := bosh.RemoteCommand(deploymentName, "mysql/0",
				"sudo cat /var/vcap/store/pxc-mysql/grastate.dat")
			Expect(err).NotTo(HaveOccurred())

			var graStateParsed map[string]any
			Expect(yaml.Unmarshal([]byte(graStateRaw), &graStateParsed)).To(Succeed())

			boshInstanceID, err := bosh.RemoteCommand(deploymentName, "mysql/0",
				"cat /var/vcap/instance/id")
			Expect(err).NotTo(HaveOccurred())

			Expect(getSeqNoParsed).To(Equal(getSeqNoResponse{
				ClusterUUID: graStateParsed["uuid"].(string),
				Seqno:       graStateParsed["seqno"].(int),
				InstanceID:  boshInstanceID,
			}))
		})

		It("can recover an offline cluster by running the bootstrap errand", func() {
			Expect(cmd.Run(
				"bosh",
				"--deployment="+deploymentName,
				"--non-interactive",
				"--tty",
				"run-errand",
				"bootstrap",
			)).To(Succeed())
		})

		It("can recover an offline cluster by running the bootstrap errand", func() {
			cmd := exec.Command("bosh",
				"--deployment="+deploymentName,
				"--non-interactive",
				"--tty",
				"run-errand",
				"bootstrap")
			out := gbytes.NewBuffer()
			cmd.Stdout = out
			cmd.Stderr = GinkgoWriter
			Expect(cmd.Run()).To(Succeed())
			Expect(out).To(gbytes.Say("This errand only runs on the bootstrap node"))
		})

		It("observes the cluster returns to a healthy three node cluster", func() {
			var unused, clusterSize string
			Expect(db.QueryRow(`SHOW GLOBAL STATUS LIKE 'wsrep\_cluster\_size'`).
				Scan(&unused, &clusterSize)).To(Succeed())
			Expect(clusterSize).To(Equal("3"))
		})

		It("observes the cluster still retains the expected data", func() {
			mysqlIps, err := bosh.InstanceIPs(deploymentName, bosh.MatchByInstanceGroup("mysql"))
			Expect(err).NotTo(HaveOccurred())

			Expect(mysqlIps).To(HaveLen(3))
			for _, host := range mysqlIps {
				db, err := sql.Open("mysql", "test-admin:integration-tests@tcp("+host+")/pxc_release_test_db?tls=skip-verify")
				Expect(err).NotTo(HaveOccurred())
				var data string
				Expect(db.QueryRow(`SELECT test_data FROM pxc_release_test_db.bootstrap_test`).
					Scan(&data)).To(Succeed())
				Expect(data).To(Equal(deploymentName + ": data written with 3 nodes"))
			}
		})

		It("can still connect to the MySQL instance through the proxies", func() {
			var data string
			Expect(db.QueryRow(`SELECT test_data FROM pxc_release_test_db.bootstrap_test`).
				Scan(&data)).To(Succeed())
			Expect(data).To(Equal(deploymentName + ": data written with 3 nodes"))
		})
	})
})
