package scaling_test

import (
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"database/sql"
	boshdir "github.com/cloudfoundry/bosh-cli/director"
	"gopkg.in/yaml.v2"
	"os"
	helpers "specs/test_helpers"
)

func scaleDeployment(instanceCount int) error {
	director, err := helpers.BuildBoshDirector()
	if err != nil {
		return fmt.Errorf("building director: %s", err)
	}

	deployment, err := director.FindDeployment(helpers.BoshDeployment())
	if err != nil {
		return fmt.Errorf("finding deployment: %s", err)
	}

	manifestString, err := deployment.Manifest()
	if err != nil {
		return fmt.Errorf("getting manifest: %s", err)
	}
	var manifest map[string]interface{}
	err = yaml.Unmarshal([]byte(manifestString), &manifest)
	if err != nil {
		return fmt.Errorf("unmarshalling manifest: %s", err)
	}

	instanceGroups := manifest["instance_groups"].([]interface{})
	for _, instanceGroup := range instanceGroups {
		if instanceGroup.(map[interface{}]interface{})["name"] == "mysql" {
			instanceGroup.(map[interface{}]interface{})["instances"] = instanceCount
			break
		}
	}

	updatedManifest, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("marshalling manifest: %s", err)
	}

	err = deployment.Update(updatedManifest, boshdir.UpdateOpts{})
	if err != nil {
		return fmt.Errorf("deploying: %s", err)
	}

	return err
}

func verifyDataExists(expectedString string, databaseConnection *sql.DB) {
	var queryResultString string
	query := fmt.Sprintf("SELECT * FROM scaling_test_table WHERE test_data='%s'", expectedString)
	rows, err := databaseConnection.Query(query)
	Expect(err).NotTo(HaveOccurred())
	rows.Next()
	rows.Scan(&queryResultString)
	Expect(queryResultString).NotTo(BeEmpty())
}

var _ = Describe("CF PXC MySQL Scaling", func() {
	var (
		pxcConnectionString string
	)

	BeforeEach(func() {
		var mysqlUsername = os.Getenv("MYSQL_USERNAME")
		var mysqlPassword = os.Getenv("MYSQL_PASSWORD")

		pxcConnectionString = fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/",
			mysqlUsername,
			mysqlPassword,
			helpers.BoshEnvironment(),
			3306)
		databaseConnection, err := sql.Open("mysql", pxcConnectionString)
		Expect(err).NotTo(HaveOccurred())

		statement := "CREATE DATABASE IF NOT EXISTS scaling_test"
		_, err = databaseConnection.Exec(statement)
		Expect(err).NotTo(HaveOccurred())

		pxcConnectionString = fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/scaling_test",
			mysqlUsername,
			mysqlPassword,
			helpers.BoshEnvironment(),
			3306)

		statement = "USE scaling_test"
		_, err = databaseConnection.Exec(statement)
		Expect(err).NotTo(HaveOccurred())

		statement = "CREATE TABLE IF NOT EXISTS scaling_test_table (test_data varchar(255))"
		_, err = databaseConnection.Exec(statement)
		Expect(err).NotTo(HaveOccurred())

		err = scaleDeployment(3)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		databaseConnection, err := sql.Open("mysql", pxcConnectionString)
		Expect(err).NotTo(HaveOccurred())

		statement := "DROP DATABASE scaling_test"
		_, err = databaseConnection.Exec(statement)
		Expect(err).NotTo(HaveOccurred())
	})

	It("proxies failover to another node after a partition of mysql node", func() {
		databaseConnection, err := sql.Open("mysql", pxcConnectionString)
		Expect(err).NotTo(HaveOccurred())

		query := "INSERT INTO scaling_test_table VALUES('data written with 3 nodes')"
		_, err = databaseConnection.Query(query)
		Expect(err).NotTo(HaveOccurred())

		err = scaleDeployment(1)
		Expect(err).NotTo(HaveOccurred())

		verifyDataExists("data written with 3 nodes", databaseConnection)

		query = "INSERT INTO scaling_test_table VALUES('data written with 1 node')"
		_, err = databaseConnection.Query(query)
		Expect(err).NotTo(HaveOccurred())

		err = scaleDeployment(3)
		Expect(err).NotTo(HaveOccurred())

		verifyDataExists("data written with 1 node", databaseConnection)
	})

})
