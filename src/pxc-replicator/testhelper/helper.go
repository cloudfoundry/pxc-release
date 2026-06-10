// Package testhelper contains setup code for percona provided pxc-xtradbcluster images:
// https://hub.docker.com/r/percona/percona-xtradb-cluster
// https://github.com/Percona-Lab/percona-docker/tree/master/percona-xtradb-cluster-8.0
package testhelper

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/cloudfoundry/pxc-release/replicator/config"

	"github.com/testcontainers/testcontainers-go"
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

func GenerateTestData(target config.Target, dbName, tableName string, numberRows int) {
	db, err := sql.Open("mysql", target.String())
	Expect(err).ToNot(HaveOccurred())

	_, err = db.Exec(fmt.Sprintf("Create DATABASE %s;", backtick(dbName)))
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

func StartContainerInstance(name, password string) config.Target {
	ctx := context.Background()
	pxc, err := testcontainers.Run(ctx,
		fmt.Sprintf("%s:%s", image, tag),
		testcontainers.WithName(name),
		testcontainers.WithExposedPorts("3306"),
		testcontainers.WithEnv(map[string]string{
			"MYSQL_ROOT_PASSWORD": password,
			"CLUSTER_NAME":        name,
			"MYSQL_ROOT_HOST":     "%",
		}), testcontainers.WithCmdArgs("--gtid-mode=ON", "--enforce-gtid-consistency=ON"),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("3306/tcp"),
			wait.ForLog("Synchronized with group, ready for connections"),
		))
	Expect(err).ToNot(HaveOccurred())
	testcontainers.CleanupContainer(ginkgo.GinkgoTB(), pxc)
	endpoint, err := pxc.Endpoint(ctx, "tcp/3306")
	Expect(err).ToNot(HaveOccurred())
	// endpoint should be in format "tcp/3306:localhost:37853"
	outerPort := strings.Split(endpoint, ":")
	portInt, err := strconv.Atoi(outerPort[len(outerPort)-1])
	Expect(err).ToNot(HaveOccurred())
	return config.Target{
		Host: "localhost",
		Port: portInt,
		Creds: config.Creds{
			Username: "root",
			Password: password,
		},
		TLS: config.Certs{},
	}
}
