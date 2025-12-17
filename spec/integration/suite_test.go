package integration_test

import (
	"log"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"integration/internal/docker"
)

func TestUserManagement(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UserManagement Suite")
}

var (
	volumeID string
)

var _ = BeforeSuite(func() {
	Expect(mysql.SetLogger(log.New(GinkgoWriter, log.Prefix(), log.Flags()))).To(Succeed())
	volumeID = uuid.New().String()
})

func startMySQL(tag string, mysqlOptions []string, extraMounts []string) (containerID string) {
	GinkgoHelper()

	containerArgs := append([]string{
		"--pxc-maint-transition-period=0",
		"--log-error-verbosity=3",
		"--innodb-flush-method=fsync",
	}, mysqlOptions...)

	containerID = docker.RunContainer(docker.ContainerSpec{
		Image:          "percona/percona-xtradb-cluster:" + tag,
		Args:           containerArgs,
		Env:            []string{"PXC_CLUSTER_NAME=testcluster", "MYSQL_ALLOW_EMPTY_PASSWORD=1"},
		Volumes:        append([]string{volumeID + ":/var/lib/mysql"}, extraMounts...),
		Ports:          []string{"3306/tcp"},
		HealthCmd:      "mysqladmin -u root --host=127.0.0.1 ping",
		HealthInterval: "2s",
	})

	Expect(docker.WaitHealthy(containerID, 5*time.Minute)).To(Succeed())

	return containerID
}
