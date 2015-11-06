package sequence_number_test

import (
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/erikstmartin/go-testdb"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/sequence_number"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("GaleraSequenceChecker", func() {

	Describe("Check", func() {
		Context("db works", func() {
			It("returns sequence number and no error", func() {
				_, err := sequenceNumberTestHelper()
				Expect(err).ToNot(HaveOccurred())
			})
		})
		Context("db is down", func() {
			It("returns an error", func() {
				_, err := sequenceNumberFailingDbSetup()
				Expect(err).To(HaveOccurred())
			})
		})
	})
})

func newTestConfig() config.Config {

	rawConfig := `{
		"StatusEndpoint": "fake",
		"Host": "localhost",
		"Port": "8080",
		"AvailableWhenReadOnly": false,
		"AvailableWhenDonor": true,
		"DB": {
			"Host": "localhost",
			"User": "vcap",
			"Port": 3000,
			"Password": "password"
		}
	}`

	osArgs := []string{
		"galera-healthcheck",
		fmt.Sprintf("-config=%s", rawConfig),
	}

	var err error
	rootConfig, err := config.NewConfig(osArgs)
	Expect(err).ToNot(HaveOccurred())
	return *rootConfig
}
func sequenceNumberFailingDbSetup() (int, error) {
	testdb.SetOpenFunc(func(dsn string) (driver.Conn, error) {
		return testdb.Conn(), errors.New("failed to connect")
	})
	db, _ := sql.Open("testdb", "")
	fake_result := "asdf:32"
	sql := "SHOW variables LIKE 'wsrep_start_position'"
	columns := []string{"Variable_name", "Value:Id"}
	result := fmt.Sprintf("wsrep_start_position,%s", fake_result)
	testdb.StubQuery(sql, testdb.RowsFromCSVString(columns, result))
	logger := lagertest.NewTestLogger("sequence_number test")
	sequence_number_checker := sequence_number.New(db, newTestConfig(), logger)

	return sequence_number_checker.Check()

}

func sequenceNumberTestHelper() (int, error) {
	db, _ := sql.Open("testdb", "")
	fake_result := "asdf:32"
	sql := "SHOW variables LIKE 'wsrep_start_position'"
	columns := []string{"Variable_name", "Value:Id"}
	result := fmt.Sprintf("wsrep_start_position,%s", fake_result)
	testdb.StubQuery(sql, testdb.RowsFromCSVString(columns, result))
	logger := lagertest.NewTestLogger("sequence_number test")
	sequence_number_checker := sequence_number.New(db, newTestConfig(), logger)

	return sequence_number_checker.Check()

}
