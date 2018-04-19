package bootstrap_test

import (
	"bytes"
	"fmt"
	boshdir "github.com/cloudfoundry/bosh-cli/director"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"golang.org/x/crypto/ssh"
	"os"
	helpers "specs/test_helpers"
	"time"
)

func runCmdOnBoshDirectorVM(cmd string) {
	boshKeySigner, err := ssh.ParsePrivateKey(helpers.BoshGwPrivateKey())
	Expect(err).NotTo(HaveOccurred())

	config := &ssh.ClientConfig{
		User: helpers.BoshGwUser(),
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(boshKeySigner),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	client, err := ssh.Dial(
		"tcp",
		fmt.Sprintf("%s:22", helpers.BoshEnvironment()),
		config,
	)
	Expect(err).NotTo(HaveOccurred())

	session, err := client.NewSession()
	Expect(err).NotTo(HaveOccurred())

	defer session.Close()

	var e bytes.Buffer
	session.Stderr = &e

	err = session.Run(cmd)
	if err != nil {
		fmt.Println(fmt.Sprintf("Output from stderr of cmd: %s", e.String()))
		Expect(err).NotTo(HaveOccurred())
	}

}

func stopGaleraInitOnAllMysqls() {
	director, err := helpers.BuildBoshDirector()
	Expect(err).NotTo(HaveOccurred())

	deployment, err := director.FindDeployment(helpers.BoshDeployment())
	Expect(err).NotTo(HaveOccurred())

	instances, err := deployment.Instances()
	Expect(err).NotTo(HaveOccurred())

	galeraAgentUsername := os.Getenv("GALERA_AGENT_USERNAME")
	galeraAgentPassword := os.Getenv("GALERA_AGENT_PASSWORD")
	for _, instance := range instances {
		if instance.Group == "mysql" {
			mysqlIp := instance.IPs[0]

			curlCmd := fmt.Sprintf("curl -vvvkf -X POST %s:%s@%s:9200/stop_mysql", galeraAgentUsername, galeraAgentPassword, mysqlIp)
			runCmdOnBoshDirectorVM(curlCmd)
		}
	}

	Eventually(func() bool {
		backend, err := helpers.ActiveProxyBackend()
		Expect(err).NotTo(HaveOccurred())

		return backend == ""
	}, 3*time.Minute, 5*time.Second).Should(BeTrue())
}

func bootstrapCluster() {
	director, err := helpers.BuildBoshDirector()
	Expect(err).NotTo(HaveOccurred())

	deployment, err := director.FindDeployment(helpers.BoshDeployment())
	Expect(err).NotTo(HaveOccurred())

	slugList := []boshdir.InstanceGroupOrInstanceSlug{boshdir.NewInstanceGroupOrInstanceSlug("mysql", "0")}
	errandResult, err := deployment.RunErrand("bootstrap", false, false, slugList)
	Expect(err).NotTo(HaveOccurred())

	fmt.Println(fmt.Sprintf("Errand STDOUT: %s", errandResult[0].Stdout))
	fmt.Println(fmt.Sprintf("Errand STDERR: %s", errandResult[0].Stderr))
}

var _ = Describe("CF PXC MySQL Bootstrap", func() {
	BeforeEach(func() {
		helpers.DbSetup("bootstrap_test_table")
	})

	AfterEach(func() {
		helpers.DbCleanup()
	})
	It("bootstraps a cluster", func() {
		By("Write data")
		dbConn := helpers.DbConn()

		query := "INSERT INTO bootstrap_test_table VALUES('the only data')"
		_, err := dbConn.Query(query)
		Expect(err).NotTo(HaveOccurred())

		stopGaleraInitOnAllMysqls()

		By("Wait for monit to finish stopping")
		time.Sleep(5 * time.Second)

		bootstrapCluster()

		By("Verify cluster has three nodes")
		var variableName, variableValue string
		query = "SHOW status LIKE 'wsrep_cluster_size'"
		rows, err := dbConn.Query(query)
		Expect(err).NotTo(HaveOccurred())

		rows.Next()
		rows.Scan(&variableName, &variableValue)

		Expect(variableValue).To(Equal("3"))

		By("Verify data still exists")
		var queryResultString string
		query = "SELECT * FROM bootstrap_test_table"
		rows, err = dbConn.Query(query)
		Expect(err).NotTo(HaveOccurred())

		rows.Next()
		rows.Scan(&queryResultString)

		Expect(queryResultString).To(Equal("the only data"))
	})

})
