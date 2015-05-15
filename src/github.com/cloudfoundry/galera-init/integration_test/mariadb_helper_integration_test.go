package integration_test

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	os_fakes "github.com/cloudfoundry/mariadb_ctrl/os_helper/fakes"
	_ "github.com/go-sql-driver/mysql"
	"github.com/nu7hatch/gouuid"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MariaDB Helper", func() {
	var (
		helper     *mariadb_helper.MariaDBHelper
		fakeOs     *os_fakes.FakeOsHelper
		testLogger lagertest.TestLogger
		logFile    string
		dbConfig   config.DBHelper
		db         *sql.DB
	)

	var openDBConnection = func(testConfig TestDBConfig) (*sql.DB, error) {
		return sql.Open("mysql", fmt.Sprintf(
			"%s:%s@tcp(%s:%d)/%s",
			testConfig.User,
			testConfig.Password,
			testConfig.Host,
			testConfig.Port,
			testConfig.DBName,
		))
	}

	var openRootDBConnection = func(testConfig TestDBConfig) (*sql.DB, error) {
		testConfig.DBName = ""
		return openDBConnection(testConfig)
	}

	BeforeEach(func() {
		// MySQL mandates usernames are <= 16 chars
		user0 := getUUIDWithPrefix("MARIADB")[:16]
		user1 := getUUIDWithPrefix("MARIADB")[:16]

		dbConfig = config.DBHelper{
			User:     testConfig.User,
			Password: testConfig.Password,
			PreseededDatabases: []config.PreseededDatabase{
				config.PreseededDatabase{
					DBName:   getUUIDWithPrefix("MARIADB_CTRL_DB"),
					User:     user0,
					Password: "password0",
				},
				config.PreseededDatabase{
					DBName:   getUUIDWithPrefix("MARIADB_CTRL_DB"),
					User:     user0,
					Password: "password0",
				},
				config.PreseededDatabase{
					DBName:   getUUIDWithPrefix("MARIADB_CTRL_DB"),
					User:     user1,
					Password: "password1",
				},
			},
		}

		fakeOs = new(os_fakes.FakeOsHelper)
		testLogger = *lagertest.NewTestLogger("mariadb_helper")
		logFile = "/log-file.log"
	})

	JustBeforeEach(func() {
		helper = mariadb_helper.NewMariaDBHelper(
			fakeOs,
			dbConfig,
			logFile,
			testLogger,
		)

		//override db connection to use test DB
		mariadb_helper.OpenDBConnection = func(config config.DBHelper) (*sql.DB, error) {
			return openRootDBConnection(testConfig)
		}

		var err error
		db, err = openRootDBConnection(testConfig)
		Expect(err).NotTo(HaveOccurred())

		err = db.Ping()
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {

		defer db.Close()

		for _, preseededDB := range dbConfig.PreseededDatabases {
			_, err := db.Exec(
				fmt.Sprintf("DROP DATABASE IF EXISTS %s", preseededDB.DBName))
			testLogger.Error("Error cleaning up test DB's", err)

			_, err = db.Exec(
				fmt.Sprintf("DROP USER %s", preseededDB.User))
			testLogger.Error("Error cleaning up test users", err)
		}
	})

	Describe("Seed", func() {

		var ensureSeedSucceeds = func() {
			err := helper.Seed()
			Expect(err).NotTo(HaveOccurred())

			for _, preseededDB := range dbConfig.PreseededDatabases {
				//check that DB exists
				dbRows, err := db.Query(fmt.Sprintf("SHOW DATABASES LIKE '%s'", preseededDB.DBName))
				Expect(err).NotTo(HaveOccurred())
				Expect(dbRows.Err()).NotTo(HaveOccurred())
				Expect(dbRows.Next()).To(BeTrue(), fmt.Sprintf("Expected DB to exist: %s", preseededDB.DBName))

				//check that user can login to DB
				userDb, err := openDBConnection(TestDBConfig{
					Host:     testConfig.Host,
					Port:     testConfig.Port,
					User:     preseededDB.User,
					Password: preseededDB.Password,
					DBName:   preseededDB.DBName,
				})
				Expect(err).NotTo(HaveOccurred())
				defer userDb.Close()

				//check that user has permission to create a table
				_, err = userDb.Exec("CREATE TABLE testTable ( ID int )")
				Expect(err).NotTo(HaveOccurred())
			}
		}

		It("seeds databases and users", ensureSeedSucceeds)

		Context("when database name contains a hyphen", func() {

			BeforeEach(func() {
				dbNameWithHyphen := getUUIDWithPrefix("MARIADB_CTRL_DB")
				dbNameWithHyphen = strings.Replace(dbNameWithHyphen, "_", "-", -1)

				dbConfig.PreseededDatabases[0].DBName = dbNameWithHyphen
			})

			It("seeds databases and users", ensureSeedSucceeds)
		})

		Context("when database name contains a hyphen", func() {

			BeforeEach(func() {
				userWithHyphen := getUUIDWithPrefix("MARIADB")[:16]
				userWithHyphen = strings.Replace(userWithHyphen, "_", "-", -1)

				dbConfig.PreseededDatabases[0].User = userWithHyphen
			})

			It("seeds databases and users", ensureSeedSucceeds)
		})

	})
})

func getUUIDWithPrefix(prefix string) string {
	id, err := uuid.NewV4()
	Expect(err).ToNot(HaveOccurred())
	idString := fmt.Sprintf("%s_%s", prefix, id.String())
	// mysql does not like hyphens in DB names
	return strings.Replace(idString, "-", "_", -1)
}
