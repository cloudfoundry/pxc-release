package integration_test

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"code.cloudfoundry.org/lager/lagertest"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper/os_helperfakes"
	"github.com/go-sql-driver/mysql"
	"github.com/nu7hatch/gouuid"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MariaDB Helper", func() {
	Describe("Seed", func() {
		var (
			helper     *mariadb_helper.MariaDBHelper
			fakeOs     *os_helperfakes.FakeOsHelper
			testLogger lagertest.TestLogger
			logFile    string
			dbConfig   *config.DBHelper
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
			databaseA := getUUIDWithPrefix("MARIADB_CTRL_DB")
			databaseB := getUUIDWithPrefix("MARIADB_CTRL_DB")

			dbConfig = &config.DBHelper{
				User:     testConfig.User,
				Password: testConfig.Password,
				// Same user for multiple databases, and same database for multiple users
				PreseededDatabases: []config.PreseededDatabase{
					config.PreseededDatabase{
						DBName:   databaseA,
						User:     user0,
						Password: "password0",
					},
					config.PreseededDatabase{
						DBName:   databaseB,
						User:     user0,
						Password: "password0",
					},
					config.PreseededDatabase{
						DBName:   databaseB,
						User:     user1,
						Password: "password1",
					},
				},
			}

			fakeOs = new(os_helperfakes.FakeOsHelper)
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
			mariadb_helper.OpenDBConnection = func(config *config.DBHelper) (*sql.DB, error) {
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
					fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", preseededDB.DBName))
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(
					fmt.Sprintf("DROP USER IF EXISTS '%s'", preseededDB.User))
				Expect(err).NotTo(HaveOccurred())
			}
		})

		Context("Seeding databases and users", func() {
			const mysqlAccessDenied uint16 = 1044
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

					//check that user has CREATE permission
					_, err = userDb.Exec("CREATE TABLE testTable ( ID int )")
					Expect(err).NotTo(HaveOccurred())

					//check that user has INSERT permission
					_, err = userDb.Exec("INSERT INTO testTable (ID) VALUES (1)")

					//check that user does not have LOCK TABLES permission
					_, err = userDb.Exec("LOCK TABLES testTable READ")
					Expect(err).To(HaveOccurred())
					e, found := err.(*mysql.MySQLError)
					Expect(found).To(BeTrue())
					Expect(e.Number).To(Equal(mysqlAccessDenied))

					//check that user has DROP permission
					_, err = userDb.Exec("DROP TABLE testTable")
					Expect(err).NotTo(HaveOccurred())
				}
			}

			It("seeds databases and users the first time", func() {
				ensureSeedSucceeds()
			})

			It("updates users if they are re-seeded with different passwords during a subsequent deploy", func() {
				ensureSeedSucceeds()
				dbConfig.PreseededDatabases[0].Password = "reseeded-password0"
				dbConfig.PreseededDatabases[1].Password = "reseeded-password0"
				dbConfig.PreseededDatabases[2].Password = "reseeded-password1"
				ensureSeedSucceeds()
			})

			Context("when database name contains a hyphen", func() {
				BeforeEach(func() {
					dbNameWithHyphen := getUUIDWithPrefix("MARIADB_CTRL_DB")
					dbNameWithHyphen = strings.Replace(dbNameWithHyphen, "_", "-", -1)

					dbConfig.PreseededDatabases[0].DBName = dbNameWithHyphen
				})

				It("seeds databases and users", func() {
					ensureSeedSucceeds()
				})
			})

			Context("when user name contains a hyphen", func() {
				BeforeEach(func() {
					userWithHyphen := getUUIDWithPrefix("MARIADB")[:16]
					userWithHyphen = strings.Replace(userWithHyphen, "_", "-", -1)

					dbConfig.PreseededDatabases[0].User = userWithHyphen
				})

				It("seeds databases and users", func() {
					ensureSeedSucceeds()
				})
			})
		})
	})

	Describe("TestDatabaseCleanup", func() {
		var (
			helper     *mariadb_helper.MariaDBHelper
			fakeOs     *os_helperfakes.FakeOsHelper
			testLogger lagertest.TestLogger
			logFile    string
			dbConfig   *config.DBHelper
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

		var testDatabaseNames = func(db *sql.DB) []string {
			rows, err := db.Query("SHOW DATABASES LIKE 'test%'")
			Expect(err).NotTo(HaveOccurred())

			var names []string
			defer rows.Close()
			for rows.Next() {
				var name string

				err := rows.Scan(&name)
				Expect(err).NotTo(HaveOccurred())

				names = append(names, name)
			}
			Expect(rows.Err()).NotTo(HaveOccurred())

			return names
		}

		BeforeEach(func() {
			fakeOs = new(os_helperfakes.FakeOsHelper)
			testLogger = *lagertest.NewTestLogger("mariadb_helper")
			logFile = "/log-file.log"

			helper = mariadb_helper.NewMariaDBHelper(
				fakeOs,
				dbConfig,
				logFile,
				testLogger,
			)

			//override db connection to use test DB
			mariadb_helper.OpenDBConnection = func(config *config.DBHelper) (*sql.DB, error) {
				return openRootDBConnection(testConfig)
			}

			var err error
			db, err = openRootDBConnection(testConfig)
			Expect(err).NotTo(HaveOccurred())

			err = db.Ping()
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			db.Close()
		})

		Context("removing permissions", func() {
			BeforeEach(func() {
				_, err := db.Exec("CREATE DATABASE IF NOT EXISTS test")
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec("CREATE TABLE IF NOT EXISTS test.foo (id int)")
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec("CREATE DATABASE IF NOT EXISTS test2")
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec("CREATE TABLE IF NOT EXISTS test2.foo (id int)")
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec("CREATE DATABASE IF NOT EXISTS test_foo")
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec("CREATE TABLE IF NOT EXISTS test_foo.foo (id int)")
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec("CREATE DATABASE IF NOT EXISTS foobar")
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec("CREATE TABLE IF NOT EXISTS foobar.foo (id int)")
				Expect(err).NotTo(HaveOccurred())

				createUserStatement := fmt.Sprintf("GRANT SELECT ON foobar.* TO 'new-user'@'%s' IDENTIFIED BY 'password'", testConfig.Host)
				_, err = db.Exec(createUserStatement)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`INSERT IGNORE INTO mysql.db VALUES ('%','test','','Y','Y','Y','Y',
				'Y','Y','N','Y','Y','Y','Y','Y','Y','Y','Y','N','N','Y','Y')`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`INSERT IGNORE INTO mysql.db VALUES ('%','test\_%','','Y','Y','Y','Y',
			'Y','Y','N','Y','Y','Y','Y','Y','Y','Y','Y','N','N','Y','Y')`)
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec(`FLUSH PRIVILEGES`)
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				_, err := db.Exec("DROP DATABASE IF EXISTS test")
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec("DROP DATABASE IF EXISTS test2")
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec("DROP DATABASE IF EXISTS test_foo")
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec("DROP DATABASE IF EXISTS foobar")
				Expect(err).NotTo(HaveOccurred())
				_, err = db.Exec("DROP USER IF EXISTS 'new-user'")
				Expect(err).NotTo(HaveOccurred())
			})

			It("removes permissions for unrelated users to test databases", func(done Done) {
				cfg := &mysql.Config{
					User:   "new-user",
					Passwd: "password",
					Net:    "tcp",
					Addr:   testConfig.Host + ":" + strconv.Itoa(testConfig.Port),
				}

				newUserConn, err := sql.Open("mysql", cfg.FormatDSN())
				Expect(err).NotTo(HaveOccurred())

				var id int
				Expect(newUserConn.QueryRow("SELECT * FROM foobar.foo").Scan(&id)).To(MatchError(sql.ErrNoRows))
				Expect(newUserConn.QueryRow("SELECT * FROM test.foo").Scan(&id)).To(MatchError(sql.ErrNoRows))
				Expect(newUserConn.QueryRow("SELECT * FROM test_foo.foo").Scan(&id)).To(MatchError(sql.ErrNoRows))
				Expect(newUserConn.QueryRow("SELECT * FROM test2.foo").Scan(&id)).To(MatchError(ContainSubstring("SELECT command denied to user 'new-user'")))

				Expect(helper.TestDatabaseCleanup()).To(Succeed())
				Expect(helper.TestDatabaseCleanup()).To(Succeed()) // Should be idempotent

				Expect(newUserConn.QueryRow("SELECT * FROM foobar.foo").Scan(&id)).To(MatchError(sql.ErrNoRows))
				Expect(newUserConn.QueryRow("SELECT * FROM test.foo").Scan(&id)).To(MatchError(ContainSubstring("SELECT command denied to user 'new-user'")))
				Expect(newUserConn.QueryRow("SELECT * FROM test_foo.foo").Scan(&id)).To(MatchError(ContainSubstring("SELECT command denied to user 'new-user'")))
				Expect(newUserConn.QueryRow("SELECT * FROM test2.foo").Scan(&id)).To(MatchError(ContainSubstring("SELECT command denied to user 'new-user'")))

				close(done)
			})

		})

		Context("removing databases", func() {
			BeforeEach(func() {
				_, err := db.Exec("CREATE DATABASE IF NOT EXISTS test")
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec("CREATE DATABASE IF NOT EXISTS test_foo")
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec("CREATE DATABASE IF NOT EXISTS test2")
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				_, err := db.Exec("DROP DATABASE IF EXISTS test")
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec("DROP DATABASE IF EXISTS test_foo")
				Expect(err).NotTo(HaveOccurred())

				_, err = db.Exec("DROP DATABASE IF EXISTS test2")
				Expect(err).NotTo(HaveOccurred())
			})

			It("removes 'test' and 'test_%' databases", func(done Done) {
				names := testDatabaseNames(db)

				Expect(names).To(ContainElement("test"))
				Expect(names).To(ContainElement("test_foo"))
				Expect(names).To(ContainElement("test2"))

				Expect(helper.TestDatabaseCleanup()).To(Succeed())
				Expect(helper.TestDatabaseCleanup()).To(Succeed()) // Should be idempotent

				names = testDatabaseNames(db)

				Expect(names).NotTo(ContainElement("test"))
				Expect(names).NotTo(ContainElement("test_foo"))
				Expect(names).To(ContainElement("test2"))

				close(done)
			})
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
