package healthcheck_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"fmt"
	"testing"

	"os"
	"os/exec"
    "time"
)

var binaryPath string

func TestHealthcheck(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Healthcheck Suite")
}

var _ = BeforeSuite(func() {
    var err error
	binaryPath, err = gexec.Build("github.com/cloudfoundry-incubator/galera-healthcheck", "-race")
	Expect(err).ToNot(HaveOccurred())

	_, err = os.Stat(binaryPath)
	if err != nil {
		Expect(os.IsExist(err)).To(BeTrue())
	}
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()

	_, err := os.Stat(binaryPath)
	if err != nil {
		Expect(os.IsExist(err)).To(BeFalse())
	}
})

func startHealthcheck(flags ...string) *gexec.Session {
    flags = append(flags, fmt.Sprintf("-port=%d", port()))

    dbHost := os.Getenv("DB_HOST")
    if dbHost != "" {
        flags = append(flags, fmt.Sprintf("-dbHost=%s", dbHost))
    }

    dbPort := os.Getenv("DB_PORT")
    if dbPort != "" {
        flags = append(flags, fmt.Sprintf("-dbPort=%s", dbPort))
    }

    dbUser := os.Getenv("DB_USER")
    if dbUser != "" {
        flags = append(flags, fmt.Sprintf("-dbUser=%s", dbUser))
    }

    dbPassword := os.Getenv("DB_PASSWORD")
    if dbPassword != "" {
        flags = append(flags, fmt.Sprintf("-dbPassword=%s", dbPassword))
    }

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