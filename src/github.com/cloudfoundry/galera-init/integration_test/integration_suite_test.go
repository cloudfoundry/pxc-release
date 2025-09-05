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
	"github.com/cloudfoundry/galera-init/internal/testing/docker"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Integration Test Suite")
}

const (
	pxcDockerImage              = "percona/percona-xtradb-cluster:8.0"
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
	_ = mysql.SetLogger(log.New(GinkgoWriter, "[mysql] ", log.Ldate|log.Ltime|log.Lshortfile))

	var err error
	galeraInitPath, err = gexec.BuildWithEnvironment(
		"github.com/cloudfoundry/galera-init/cmd/start/",
		[]string{
			"GOOS=linux",
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
	sessionTmpdir, err = os.MkdirTemp("", "_galera_init_integration")
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		_ = os.RemoveAll(sessionTmpdir)
	})
	Expect(os.Chmod(sessionTmpdir, 0777)).To(Succeed())

	dockerNetwork = "mysql-net." + sessionID
	docker.CreateNetwork(dockerNetwork)
	DeferCleanup(func() {
		docker.RemoveNetwork(dockerNetwork)
	})
})

func mustAbsPath(path string) string {
	abspath, err := filepath.Abs(path)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return abspath
}

type ContainerOption func(cfg *docker.ContainerSpec)

func AddEnvVars(envVars ...string) ContainerOption {
	return func(spec *docker.ContainerSpec) {
		spec.Env = append(spec.Env, envVars...)
	}
}

func createGaleraContainer(
	name string,
	cfg config.Config,
	options ...ContainerOption,
) (containerID string) {
	marshalledConfig, err := json.Marshal(&cfg)
	Expect(err).NotTo(HaveOccurred(),
		`Failed to marshal galera-init config to JSON: %s`, err)

	containerSpec := docker.ContainerSpec{
		Name:       name + "." + sessionID,
		Image:      pxcDockerImage,
		Entrypoint: "docker_entrypoint.sh",
		Network:    dockerNetwork,
		Ports:      []string{pxcMySQLPort, galeraInitStatusPort},
		Volumes: []string{
			galeraInitPath + ":/usr/local/bin/galera-init",
			// galera-init currently embeds this /var/vcap path internally
			sessionTmpdir + ":" + "/var/vcap/jobs/pxc-mysql/config/",
			mustAbsPath("fixtures/docker_entrypoint.sh:/usr/local/bin/docker_entrypoint.sh"),
			mustAbsPath("fixtures/init.sql:/usr/local/etc/init.sql"),
			mustAbsPath("fixtures/my.cnf.template:/usr/local/etc/my.cnf.template"),
		},
		Env: []string{
			"CONFIG=" + string(marshalledConfig),
			"WSREP_CLUSTER_ADDRESS=gcomm://" + name + "." + sessionID,
			"WSREP_NODE_ADDRESS=" + name + "." + sessionID + ":4567",
			"WSREP_NODE_NAME=" + name,
		},
	}

	for _, opt := range options {
		opt(&containerSpec)
	}

	return docker.RunContainer(containerSpec)
}

func serviceStatus(containerID string) error {
	serviceHealthyPort := docker.ContainerPort(containerID, galeraInitStatusPort)
	res, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s", serviceHealthyPort))
	if err != nil || res.StatusCode != http.StatusOK {
		return errors.New("galera-init not healthy")
	}
	return nil
}
