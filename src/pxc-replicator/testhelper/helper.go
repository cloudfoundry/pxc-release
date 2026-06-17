// Package testhelper contains setup code for percona provided pxc-xtradbcluster images:
// https://hub.docker.com/r/percona/percona-xtradb-cluster
// https://github.com/Percona-Lab/percona-docker/tree/master/percona-xtradb-cluster-8.0
package testhelper

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"github.com/cloudfoundry/pxc-release/replicator/config"
	_ "github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	image = "percona/percona-xtradb-cluster"
	tag   = "8.4"
)

func backtick(in string) string {
	return fmt.Sprintf("`%s`", in)
}

func CreateTestNetwork() (*testcontainers.DockerNetwork, []string) {
	ctx := context.Background()
	newNetwork, err := network.New(ctx)
	Expect(err).ToNot(HaveOccurred())
	testcontainers.CleanupNetwork(ginkgo.GinkgoTB(), newNetwork)

	aliases := []string{uuid.New().String()}

	return newNetwork, aliases
}

func GenerateTestData(target config.Target, dbName, tableName string, numberRows int) {
	db, err := sql.Open("mysql", target.String())
	Expect(err).ToNot(HaveOccurred())

	_, err = db.Exec(fmt.Sprintf("Create DATABASE IF NOT EXISTS %s;", backtick(dbName)))
	Expect(err).ToNot(HaveOccurred())
	Expect(db.Close()).To(Succeed())

	db, err = sql.Open("mysql", fmt.Sprintf("%s%s", target.String(), dbName))
	Expect(err).ToNot(HaveOccurred())
	_, err = db.Exec(fmt.Sprintf(`CREATE TABLE %s (
    id INT AUTO_INCREMENT PRIMARY KEY,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  	updated_at DATETIME DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		value INT NOT NULL
	);`, backtick(tableName)))
	Expect(err).ToNot(HaveOccurred())
	for i := range numberRows {
		_, err = db.Exec(fmt.Sprintf("INSERT INTO `%s` VALUES(NULL,NULL,NULL,?);", tableName), i)
		Expect(err).ToNot(HaveOccurred())
	}
}

type StdoutLogConsumer struct{}

func (lc *StdoutLogConsumer) Accept(l testcontainers.Log) {
	log.Default().Println("mysql:", string(l.Content))
}

type Log struct {
	LogType string
	Content []byte
}

func StartContainerInstance(name, password string, netAliases []string, net *testcontainers.DockerNetwork) (fromContainer config.Target, fromHost config.Target) {
	ctx := context.Background()
	serverID := rand.Intn(999) + 1
	opts := []testcontainers.ContainerCustomizer{
		network.WithNetwork(netAliases, net),
		testcontainers.WithExposedPorts("3306"),
		testcontainers.WithName(name),
		testcontainers.WithEnv(map[string]string{
			"MYSQL_ROOT_PASSWORD": password,
			"CLUSTER_NAME":        name,
			"MYSQL_ROOT_HOST":     "%",
		}),
		testcontainers.WithCmdArgs("--gtid-mode=ON", "--enforce-gtid-consistency=ON", "--pxc_strict_mode=PERMISSIVE", fmt.Sprintf("--server-id=%d", serverID)),
		testcontainers.WithWaitStrategy(
			wait.ForLog("Synchronized with group, ready for connections").WithStartupTimeout(180*time.Second),
			wait.ForListeningPort("3306/tcp").WithStartupTimeout(120*time.Second),
			wait.ForExposedPort().WithStartupTimeout(120*time.Second),
		),
	}
	if os.Getenv("TEST_DEBUG") == "true" {
		opts = append(opts, testcontainers.WithLogConsumerConfig(&testcontainers.LogConsumerConfig{
			Opts:      []testcontainers.LogProductionOption{testcontainers.WithLogProductionTimeout(10 * time.Second)},
			Consumers: []testcontainers.LogConsumer{&StdoutLogConsumer{}},
		}),
		)
	}
	pxc, err := testcontainers.Run(ctx, fmt.Sprintf("%s:%s", image, tag), opts...)

	Expect(err).ToNot(HaveOccurred())
	testcontainers.CleanupContainer(ginkgo.GinkgoTB(), pxc, testcontainers.StopTimeout(120*time.Second))
	ip, err := pxc.ContainerIP(context.Background())
	Expect(err).ToNot(HaveOccurred())
	port, err := pxc.MappedPort(context.Background(), "3306")
	Expect(err).ToNot(HaveOccurred())

	// the networking with testcontainers makes this a bit hard.. we need to configure the replica with the "inner view" using the ContainerIP and the default 3306 port
	// but to run external checks we need the Host view which is a mapped port on localhost...

	return config.Target{
			Name: name,
			Host: ip,
			Port: 3306,
			Creds: config.Creds{
				Username: "root",
				Password: password,
			},
			TLS: config.Certs{},
		}, config.Target{
			Name: name,
			Host: "localhost",
			Port: port.Num(),
			Creds: config.Creds{
				Username: "root",
				Password: password,
			},
		}
}
