package integration_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/integration_test/docker"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Integration Test Suite")
}

const (
	pxcMySQLPort         string = "3306/tcp"
	galeraInitStatusPort string = "8114/tcp"
)

var (
	dockerNetwork  string
	sessionID      string
	sessionTmpdir  string
	galeraInitPath string
)

var _ = BeforeSuite(func() {
	log.SetOutput(GinkgoWriter)
	mysql.SetLogger(log.New(GinkgoWriter, "[mysql] ", log.Ldate|log.Ltime|log.Lshortfile))

	var err error

	galeraInitPath, err = gexec.BuildWithEnvironment(
		"github.com/cloudfoundry/galera-init/cmd/start/",
		[]string{
			"GOOS=linux",
			"GOARCH=amd64",
			"CGO_ENABLED=0",
		},
	)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})

var _ = BeforeEach(func() {
	sessionID = uuid.New().String()
	var err error
	sessionTmpdir, err = os.MkdirTemp(os.TempDir(), "_galera_init_integration")
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Chmod(sessionTmpdir, 0777)).To(Succeed())

	dockerNetwork = "mysql-net." + sessionID
	err = docker.CreateNetwork(dockerNetwork)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterEach(func() {
	if sessionTmpdir != "" {
		_ = os.RemoveAll(sessionTmpdir)
	}

	Expect(docker.RemoveNetwork(dockerNetwork)).To(Succeed())
})

func mustAbsPath(path string) string {
	abspath, err := filepath.Abs(path)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return abspath
}

func createGaleraContainer(
	name string,
	cfg config.Config,
	envVars ...string,
) (string, error) {
	GinkgoHelper()

	marshalledConfig, err := json.Marshal(&cfg)
	if err != nil {
		return "", errors.New("failed to marshal configuration")
	}

	env := []string{
		"MYSQL_ALLOW_EMPTY_PASSWORD=1",
		"PXC_CLUSTER_NAME=galera",
		"CONFIG=" + string(marshalledConfig),
		"WSREP_CLUSTER_ADDRESS=gcomm://" + name + "." + sessionID,
		"WSREP_NODE_ADDRESS=" + name + "." + sessionID + ":4567",
		"WSREP_NODE_NAME=" + name,
	}
	env = append(env, envVars...)

	container, err := docker.RunContainer(docker.ContainerSpec{
		Image:          "percona/percona-xtradb-cluster:8.0",
		Env:            env,
		HealthCmd:      "mysqladmin -u root --host=127.0.0.1 ping",
		HealthInterval: "2s",
		Ports:          []string{pxcMySQLPort, galeraInitStatusPort},
		Volumes: []string{
			galeraInitPath + ":/usr/local/bin/galera-init",
			sessionTmpdir + ":" + "/var/vcap/jobs/pxc-mysql/config/",
			mustAbsPath("fixtures/docker_entrypoint.sh:/usr/local/bin/docker_entrypoint.sh"),
			mustAbsPath("fixtures/init.sql:/usr/local/etc/init.sql"),
			mustAbsPath("fixtures/my.cnf.template:/usr/local/etc/my.cnf.template"),
		},
		Entrypoint: "docker_entrypoint.sh",
		Network:    dockerNetwork,
		Name:       name + "." + sessionID,
	})
	Expect(err).NotTo(HaveOccurred(), `Failed to initialize MySQL Container`)

	return container, nil

}

func serviceStatus(container string) error {
	GinkgoHelper()
	serviceHealthyPort, err := docker.ContainerPort(container, galeraInitStatusPort)
	Expect(err).NotTo(HaveOccurred())

	res, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s", serviceHealthyPort))
	if err != nil || res.StatusCode != http.StatusOK {
		return errors.New("galera-init not healthy")
	}
	return nil
}
