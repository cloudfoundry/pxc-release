package healthcheck_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"fmt"
	"testing"

	"encoding/json"
	"github.com/cloudfoundry-incubator/galera-healthcheck/healthcheck"
	"github.com/pivotal-cf-experimental/service-config"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

var binaryPath string
var tempDir string
var configPath string

func TestHealthcheck(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Healthcheck Suite")
}

var _ = BeforeSuite(func() {
	var err error
	tempDir, err = ioutil.TempDir(os.TempDir(), fmt.Sprintf("galera-healthcheck-integration-test-%d", GinkgoParallelNode()))
	Expect(err).NotTo(HaveOccurred())

	configPath = filepath.Join(tempDir, "healthcheckConfig.yml")

	config := newConfig()
	writeConfig(config)

	binaryPath, err = gexec.Build("github.com/cloudfoundry-incubator/galera-healthcheck", "-race")
	Expect(err).ToNot(HaveOccurred())

	_, err = os.Stat(binaryPath)
	if err != nil {
		Expect(os.IsExist(err)).To(BeTrue())
	}
})

var _ = AfterSuite(func() {
	// We don't need to handle an error cleaning up the tempDir
	_ = os.RemoveAll(tempDir)

	gexec.CleanupBuildArtifacts()

	_, err := os.Stat(binaryPath)
	if err != nil {
		Expect(os.IsExist(err)).To(BeFalse())
	}
})

func newConfig() healthcheck.Config {
	serviceConfig := service_config.New()

	var config healthcheck.Config
	err := serviceConfig.Read(&config)
	Expect(err).ToNot(HaveOccurred())

	config.Port = port()

	return config
}

func writeConfig(config healthcheck.Config) {
	fileToWrite, err := os.Create(configPath)
	Expect(err).ToNot(HaveOccurred())

	bytes, err := json.Marshal(config)
	Expect(err).ToNot(HaveOccurred())

	_, err = fileToWrite.Write(bytes)
	Expect(err).ToNot(HaveOccurred())
}

func startHealthcheck(flags ...string) *gexec.Session {
	flags = append(flags, fmt.Sprintf("-configPath=%s", configPath))
	flags = append(flags, "-logLevel=debug")

	command := exec.Command(binaryPath, flags...)

	session, err := gexec.Start(command, GinkgoWriter, GinkgoWriter)
	Expect(err).ShouldNot(HaveOccurred())

	return session
}

func awaitHealthcheckStarted(session *gexec.Session) {
	Eventually(session.Out).Should(gbytes.Say("Healthcheck Started"))
}

func stopHealthcheck(session *gexec.Session) {
	session.Kill()

	// Once signalled, the session should shut down relatively quickly
	session.Wait(5 * time.Second)

	// We don't care what the exit code is
	Eventually(session).Should(gexec.Exit())
}

func port() int {
	// magic number for start of port range
	return 51100 + GinkgoParallelNode() - 1
}

func endpoint() string {
	return fmt.Sprintf("http://localhost:%d", port())
}
