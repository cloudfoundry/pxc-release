package cluster_health_logging_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

const (
	clusterHealthLogDir     = "/var/vcap/sys/log/cluster-health-logger"
	clusterHealthLogPath    = "/var/vcap/sys/log/cluster-health-logger/cluster_health.log"
	clusterHealthErrLogPath = "/var/vcap/sys/log/cluster-health-logger/cluster-health-logger.stderr.log"
)

var _ = Describe("CF PXC MySQL Cluster Health Logging", func() {

	var (
		activeBackend string
	)
	BeforeEach(func() {
		activeBackend = "mysql/1"
		enableAccessToClusterHealthLogs(activeBackend)
	})
	It("has a cluster health logging file", func() {
		destDir, err := ioutil.TempDir("", "cluster_health_log_destination")
		Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(destDir)

		fileName := filepath.Base(clusterHealthLogPath)
		destPath := fmt.Sprintf("%s/%s", destDir, fileName)

		var session *gexec.Session
		session, err = BoshSCP(activeBackend, clusterHealthLogPath, destPath)
		Expect(err).ShouldNot(HaveOccurred())
		Eventually(session, "30s", "1s").Should(gexec.Exit(0))
		Expect(destPath).To(BeARegularFile())
	})
	It("writes metrics to the cluster health logging file", func() {
		destDir, err := ioutil.TempDir("", "cluster_health_log_destination")
		Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(destDir)

		fileName := filepath.Base(clusterHealthLogPath)
		destPath := fmt.Sprintf("%s/%s", destDir, fileName)

		var session *gexec.Session
		session, err = BoshSCP(activeBackend, clusterHealthLogPath, destPath)
		Expect(err).ShouldNot(HaveOccurred())
		Eventually(session, "30s", "1s").Should(gexec.Exit(0))
		Expect(destPath).To(BeARegularFile())

		content := getFileContents(destPath)
		Expect(content).To(ContainSubstring("timestamp|wsrep_local_state_uuid|wsrep_protocol_version|wsrep_last_applied|wsrep_last_committed"))
	})
	It("Does not write errors to the stderr file", func() {
		destDir, err := ioutil.TempDir("", "cluster_health_err_destination")
		Expect(err).NotTo(HaveOccurred())
		defer os.RemoveAll(destDir)

		fileName := filepath.Base(clusterHealthErrLogPath)
		destPath := fmt.Sprintf("%s/%s", destDir, fileName)
		var session *gexec.Session
		session, err = BoshSCP(activeBackend, clusterHealthErrLogPath, destPath)
		Expect(err).ShouldNot(HaveOccurred())
		Eventually(session, "30s", "1s").Should(gexec.Exit(0))
		Expect(destPath).To(BeARegularFile())

		content := getFileContents(destPath)
		Expect(content).ToNot(ContainSubstring("Access denied for user 'cluster-health-logger'"))
	})
})

func getFileContents(destPath string) string {
	file, err := os.Open(destPath)
	Expect(err).NotTo(HaveOccurred())
	defer file.Close()
	content, err := ioutil.ReadFile(destPath)
	Expect(err).ToNot(HaveOccurred())
	return string(content)
}

// to continue to read the audit logs.
// bpm v1.1+ sets restrict permissions for mounts configured in a job. This enables access for any user in the vcap group
func enableAccessToClusterHealthLogs(backend string) {
	// TODO: Maybe we need to chmod.  Doesn't bpm set 0700 perms?
	cmd := exec.Command("bosh", "ssh", backend, "sudo chmod 777 "+clusterHealthLogDir)
	session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	session.Wait(10 * time.Second)
	cmd = exec.Command("bosh", "ssh", backend, "sudo chmod 777 "+clusterHealthErrLogPath)
	session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	session.Wait(10 * time.Second)
	cmd = exec.Command("bosh", "ssh", backend, "sudo chmod 777 "+clusterHealthLogPath)
	session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	session.Wait(10 * time.Second)
	Expect(err).NotTo(HaveOccurred())
}

// Get the size of the audit log file in bytes before reading or writing any data
// so we can read from that offset in the audit log file and return the contents from after that offset
func BoshSCP(activeBackend, remoteFilePath, destPath string) (*gexec.Session, error) {
	sourcePath := fmt.Sprintf("%s:%s", activeBackend, remoteFilePath)
	cmd := exec.Command("bosh", "scp", sourcePath, destPath)
	return gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
}
