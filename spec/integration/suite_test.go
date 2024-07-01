package integration_test

import (
	"database/sql"
	"fmt"
	"log"
	"testing"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/ory/dockertest/v3"
)

func TestUserManagement(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "UserManagement Suite")
}

var (
	pool     *dockertest.Pool
	volumeID string
)

var _ = BeforeSuite(func() {
	Expect(mysql.SetLogger(log.New(GinkgoWriter, log.Prefix(), log.Flags()))).To(Succeed())

	var err error
	pool, err = dockertest.NewPool("")
	Expect(err).NotTo(HaveOccurred())

	volumeID = uuid.New().String()
})

func startMySQL(tag string, mysqlOptions []string, extraMounts []string) (*dockertest.Resource, error) {
	resource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "percona/percona-xtradb-cluster",
		Tag:        tag,
		Cmd: append([]string{
			"--pxc-maint-transition-period=0",
			"--log-error-verbosity=3",
		}, mysqlOptions...),
		Env:          []string{"PXC_CLUSTER_NAME=testcluster", "MYSQL_ALLOW_EMPTY_PASSWORD=1"},
		Mounts:       append([]string{volumeID + ":/var/lib/mysql"}, extraMounts...),
		ExposedPorts: []string{"3306/tcp"},
	})
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("mysql", fmt.Sprintf("root@(localhost:%s)/mysql", resource.GetPort("3306/tcp")))
	if err != nil {
		resource.Close()
		return nil, err
	}
	defer func(db *sql.DB) {
		_ = db.Close()
	}(db)

	return resource, pool.Retry(db.Ping)
}
