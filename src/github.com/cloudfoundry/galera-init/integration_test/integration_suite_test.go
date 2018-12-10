package integration_test

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/fsouza/go-dockerclient"
	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
	"github.com/pivotal-cf-experimental/service-config"

	"github.com/cloudfoundry/galera-init/config"
	. "github.com/cloudfoundry/galera-init/integration_test/test_helpers"
)

var testConfig TestDBConfig

type TestDBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
}

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Integration Test Suite")
}

const (
	pxcDockerImage = "percona/percona-xtradb-cluster:5.7"
	pxcMySQLPort   = "3306/tcp"
)

var (
	dockerClient   *docker.Client
	dockerNetwork  *docker.Network
	sessionID      string
	sessionTmpdir  string
	galeraInitPath string
)

var _ = BeforeSuite(func() {
	serviceConfig := service_config.New()

	//Use default options rather than throw error if env variables are blank
	if os.Getenv("CONFIG") == "" && os.Getenv("CONFIG_PATH") == "" {
		os.Setenv("CONFIG", "{}")
	}

	serviceConfig.AddDefaults(TestDBConfig{
		Host:     "127.0.0.1",
		Port:     3306,
		User:     "root",
		Password: "",
	})
	Expect(serviceConfig.Read(&testConfig)).To(Succeed())

	log.SetOutput(GinkgoWriter)
	mysql.SetLogger(log.New(GinkgoWriter, "[mysql] ", log.Ldate|log.Ltime|log.Lshortfile))

	var err error
	dockerClient, err = docker.NewClientFromEnv()
	Expect(err).NotTo(HaveOccurred())

	Expect(PullImage(dockerClient, pxcDockerImage)).To(Succeed())

	// Hack to ensure docker can map galera-init into a container on OS X
	// (/var/folders isn't shared by default)
	os.Setenv("TMPDIR", "/tmp")

	galeraInitPath, err = gexec.BuildWithEnvironment(
		"github.com/cloudfoundry/galera-init/cmd/start/",
		[]string{
			"GOOS=linux",
			"GOARCH=amd64",
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
	sessionTmpdir, err = ioutil.TempDir(os.TempDir(), "_galera_init_integration")
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Chmod(sessionTmpdir, 0777)).To(Succeed())

	dockerNetwork, err = CreateNetwork(dockerClient, "mysql-net."+sessionID)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterEach(func() {
	if sessionTmpdir != "" {
		os.RemoveAll(sessionTmpdir)
	}

	Expect(dockerClient.RemoveNetwork(dockerNetwork.ID)).To(Succeed())

})

func mustAbsPath(path string) string {
	abspath, err := filepath.Abs(path)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return abspath
}

func createGaleraContainer(
	name string,
	cfg config.Config,
	options ...ContainerOption,
) (*docker.Container, error) {
	marshalledConfig, err := json.Marshal(&cfg)
	if err != nil {
		return nil, errors.New("failed to marshal configuration")
	}

	defaultOptions := []ContainerOption{
		AddExposedPorts(pxcMySQLPort),
		AddBinds(
			galeraInitPath+":/usr/local/bin/galera-init",
			sessionTmpdir+":"+"/var/vcap/jobs/pxc-mysql/config/",
			mustAbsPath("fixtures/docker_entrypoint.sh:/usr/local/bin/docker_entrypoint.sh"),
			mustAbsPath("fixtures/init.sql:/usr/local/etc/init.sql"),
			mustAbsPath("fixtures/my.cnf.template:/usr/local/etc/my.cnf.template"),
			mustAbsPath("fixtures/mylogin.cnf:/var/vcap/jobs/pxc-mysql/config/mylogin.cnf"),
		),
		AddEnvVars(
			"CONFIG="+string(marshalledConfig),
			"WSREP_CLUSTER_ADDRESS=gcomm://"+name+"."+sessionID,
			"WSREP_NODE_ADDRESS="+name+"."+sessionID+":4567",
			"WSREP_NODE_NAME="+name,
		),
		WithEntrypoint("docker_entrypoint.sh"),
		WithImage(pxcDockerImage),
		WithNetwork(dockerNetwork),
	}

	return RunContainer(
		dockerClient,
		name+"."+sessionID,
		append(defaultOptions, options...)...,
	)
}
