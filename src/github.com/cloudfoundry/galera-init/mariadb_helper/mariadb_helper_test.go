package mariadb_helper_test

import (
	"database/sql"
	"errors"
	"fmt"

	"io/ioutil"
	"os"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/seeder"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/seeder/seederfakes"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper/os_helperfakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
)

var _ = Describe("MariaDBHelper", func() {
	const (
		lastInsertId           = -1
		rowsAffected           = 1
		fakeSupplementalQuery1 = "some fake query"
		fakeSupplementalQuery2 = "another fake query"
	)

	var (
		helper     *mariadb_helper.MariaDBHelper
		fakeOs     *os_helperfakes.FakeOsHelper
		fakeSeeder *seederfakes.FakeSeeder
		testLogger lagertest.TestLogger
		logFile    string
		dbConfig   *config.DBHelper
		fakeDB     *sql.DB
		mock       sqlmock.Sqlmock
	)

	BeforeEach(func() {
		var err error
		fakeOs = new(os_helperfakes.FakeOsHelper)
		fakeSeeder = new(seederfakes.FakeSeeder)
		testLogger = *lagertest.NewTestLogger("mariadb_helper")

		fakeDB, mock, err = sqlmock.New()
		Expect(err).ToNot(HaveOccurred())
		mariadb_helper.OpenDBConnection = func(*config.DBHelper) (*sql.DB, error) {
			return fakeDB, nil
		}
		mariadb_helper.CloseDBConnection = func(*sql.DB) error {
			// fakeDB is closed in AfterEach to allow assertions against mock expectations
			return nil
		}

		mariadb_helper.BuildSeeder = func(db *sql.DB, config config.PreseededDatabase, logger lager.Logger) seeder.Seeder {
			return fakeSeeder
		}

		logFile = "/log-file.log"

		sqlFile1, _ := ioutil.TempFile(os.TempDir(), "fake_sql_file")
		defer sqlFile1.Close()
		sqlFile2, _ := ioutil.TempFile(os.TempDir(), "fake_sql_file")
		defer sqlFile2.Close()

		ioutil.WriteFile(sqlFile1.Name(), []byte(fakeSupplementalQuery1), 755)
		ioutil.WriteFile(sqlFile2.Name(), []byte(fakeSupplementalQuery2), 755)

		dbConfig = &config.DBHelper{
			UpgradePath: "/mysql_upgrade",
			User:        "user",
			Password:    "password",
			PreseededDatabases: []config.PreseededDatabase{
				config.PreseededDatabase{
					DBName:   "DB1",
					User:     "user1",
					Password: "password1",
				},
				config.PreseededDatabase{
					DBName:   "DB2",
					User:     "user2",
					Password: "password2",
				},
			},
			PostStartSQLFiles: []string{sqlFile1.Name(), sqlFile2.Name()},
		}
	})

	JustBeforeEach(func() {
		helper = mariadb_helper.NewMariaDBHelper(
			fakeOs,
			dbConfig,
			logFile,
			testLogger,
		)
	})

	AfterEach(func() {
		Expect(mock.ExpectationsWereMet()).To(Succeed())
	})

	Describe("StartMysqldInStandAlone", func() {
		It("calls the mysql daemon with the command option", func() {
			options := []string{
				"--wsrep-on=OFF",
				"--wsrep-desync=ON",
				"--wsrep-OSU-method=RSU",
				"--wsrep-provider=none",
				"--skip-networking",
				"--daemonize",
			}
			helper.StartMysqldInStandAlone()

			Expect(fakeOs.RunCommandCallCount()).To(Equal(1))
			executable, args := fakeOs.RunCommandArgsForCall(0)
			Expect(executable).To(Equal("mysqld"))
			Expect(args).To(Equal(options))
		})

		Context("when an error occurs while shelling out", func() {
			It("should panic", func() {
				fakeOs.RunCommandStub = func(command string, args ...string) (string, error) {
					return "", errors.New("starting somehow failed")
				}

				Expect(func() { helper.StartMysqldInStandAlone() }).Should(Panic())
			})
		})
	})

	Describe("StopMysqld", func() {
		It("calls the mysql daemon with the stop command", func() {
			fakeOs.RunCommandReturns("", nil)

			helper.StopMysqld()

			executable, args := fakeOs.RunCommandArgsForCall(0)
			Expect(executable).To(Equal("mysqladmin"))
			Expect(args).To(Equal([]string{"--defaults-file=/var/vcap/jobs/mysql/config/mylogin.cnf", "shutdown"}))
		})

		Context("when an error occurs", func() {

			It("panics with the error", func() {
				fakeOs.RunCommandStub = func(command string, args ...string) (string, error) {
					return "", errors.New("stopping somehow failed")
				}

				Expect(func() { helper.StopMysqld() }).Should(Panic())
			})
		})
	})

	Describe("IsProcessRunning", func() {
		It("returns true if `mysql.server status` exits zero", func() {
			fakeOs.RunCommandReturns("", nil)

			isRunning := helper.IsProcessRunning()
			Expect(isRunning).To(BeTrue())

			Expect(fakeOs.RunCommandCallCount()).To(Equal(1))
			executable, args := fakeOs.RunCommandArgsForCall(0)
			Expect(executable).To(Equal("mysqladmin"))
			Expect(args).To(Equal([]string{"--defaults-file=/var/vcap/jobs/mysql/config/mylogin.cnf", "status"}))
		})

		It("returns false if `mysql.server status` exits non-zero", func() {
			fakeOs.RunCommandReturns("", errors.New("error checking status"))

			isRunning := helper.IsProcessRunning()
			Expect(isRunning).To(BeFalse())

			Expect(fakeOs.RunCommandCallCount()).To(Equal(1))
			executable, args := fakeOs.RunCommandArgsForCall(0)
			Expect(executable).To(Equal("mysqladmin"))
			Expect(args).To(Equal([]string{"--defaults-file=/var/vcap/jobs/mysql/config/mylogin.cnf", "status"}))
		})
	})

	Describe("Upgrade", func() {
		It("calls the mysql upgrade script", func() {
			helper.Upgrade()
			Expect(fakeOs.RunCommandCallCount()).To(Equal(1))

			executable, args := fakeOs.RunCommandArgsForCall(0)
			Expect(executable).To(Equal(dbConfig.UpgradePath))
			Expect(args).To(Equal([]string{
				fmt.Sprintf("-u%s", dbConfig.User),
				fmt.Sprintf("-p%s", dbConfig.Password),
			}))
		})

		It("returns the output and error", func() {
			fakeOs.RunCommandReturns("some output", errors.New("some error"))

			output, err := helper.Upgrade()
			Expect(output).To(Equal("some output"))
			Expect(err.Error()).To(Equal("some error"))
		})
	})

	Describe("IsDatabaseReachable", func() {

		galeraEnabledQuery := `SHOW GLOBAL VARIABLES LIKE 'wsrep\\_on'`
		galeraReadyQuery := `SHOW STATUS LIKE 'wsrep\\_ready'`

		Describe("when galera is enabled", func() {
			BeforeEach(func() {
				mock.ExpectQuery(galeraEnabledQuery).
					WillReturnRows(sqlmock.NewRows([]string{"Variable_name", "Value"}).
						AddRow("wsrep_on", "ON"))
			})

			Describe("when the ready check fails", func() {
				BeforeEach(func() {
					mock.ExpectQuery(galeraReadyQuery).
						WillReturnError(fmt.Errorf("WHy did this fail?"))
				})

				It("returns false", func() {
					Expect(helper.IsDatabaseReachable()).To(BeFalse())
				})
			})

			Describe("when the ready check returns ON", func() {
				BeforeEach(func() {
					mock.ExpectQuery(galeraReadyQuery).
						WillReturnRows(sqlmock.NewRows([]string{"Variable_name", "Value"}).
							AddRow("wsrep_ready", "ON"))
				})

				It("returns true", func() {
					Expect(helper.IsDatabaseReachable()).To(BeTrue())
				})
			})

			Describe("when the ready check returns OFF", func() {
				BeforeEach(func() {
					mock.ExpectQuery(galeraReadyQuery).
						WillReturnRows(sqlmock.NewRows([]string{"Variable_name", "Value"}).
							AddRow("wsrep_ready", "OFF"))
				})

				It("returns false", func() {
					Expect(helper.IsDatabaseReachable()).To(BeFalse())
				})
			})
		})

		Describe("when galera is disabled", func() {
			BeforeEach(func() {
				mock.ExpectQuery(galeraEnabledQuery).
					WillReturnRows(sqlmock.NewRows([]string{"Variable_name", "Value"}).
						AddRow("wsrep_on", "OFF"))
			})

			It("returns true", func() {
				Expect(helper.IsDatabaseReachable()).To(BeTrue())
			})
		})

		Describe("when db is unreachable", func() {
			BeforeEach(func() {
				mock.ExpectQuery(galeraEnabledQuery).
					WillReturnError(fmt.Errorf("Database isn't up yet"))
			})

			It("returns false", func() {
				Expect(helper.IsDatabaseReachable()).To(BeFalse())
			})
		})

	})

	Describe("Seed", func() {
		Context("when there are pre-seeded databases", func() {
			Context("if the users already exist", func() {
				BeforeEach(func() {
					fakeSeeder.IsExistingUserReturns(true, nil)

					mock.ExpectExec("FLUSH PRIVILEGES").
						WithArgs().
						WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
				})

				It("creates the specified databases if they don't exist and updates the users", func() {
					helper.Seed()

					Expect(fakeSeeder.CreateDBIfNeededCallCount()).To(Equal(2))
					Expect(fakeSeeder.IsExistingUserCallCount()).To(Equal(2))
					Expect(fakeSeeder.CreateUserCallCount()).To(Equal(0))
					Expect(fakeSeeder.UpdateUserCallCount()).To(Equal(2))
					Expect(fakeSeeder.GrantUserPrivilegesCallCount()).To(Equal(2))
				})
			})

			Context("if the users do not exist", func() {
				BeforeEach(func() {
					fakeSeeder.IsExistingUserReturns(false, nil)

					mock.ExpectExec("FLUSH PRIVILEGES").
						WithArgs().
						WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
				})

				It("creates the specified databases if they don't exist and creates users", func() {
					helper.Seed()

					Expect(fakeSeeder.CreateDBIfNeededCallCount()).To(Equal(2))
					Expect(fakeSeeder.IsExistingUserCallCount()).To(Equal(2))
					Expect(fakeSeeder.CreateUserCallCount()).To(Equal(2))
					Expect(fakeSeeder.GrantUserPrivilegesCallCount()).To(Equal(2))
				})
			})

			Context("when a seeder function call returns an error", func() {
				It("returns the error back", func() {
					fakeSeeder.CreateDBIfNeededReturns(errors.New("Error"))
					err := helper.Seed()
					Expect(err).To(HaveOccurred())

					fakeSeeder.IsExistingUserReturns(false, errors.New("Error"))
					err = helper.Seed()
					Expect(err).To(HaveOccurred())

					fakeSeeder.CreateUserReturns(errors.New("Error"))
					err = helper.Seed()
					Expect(err).To(HaveOccurred())

					fakeSeeder.GrantUserPrivilegesReturns(errors.New("Error"))
					err = helper.Seed()
					Expect(err).To(HaveOccurred())

					fakeSeeder.UpdateUserReturns(errors.New("Error"))
					err = helper.Seed()
					Expect(err).To(HaveOccurred())
				})
			})

		})

		Context("when there are no seeded databases", func() {
			BeforeEach(func() {
				dbConfig.PreseededDatabases = []config.PreseededDatabase{}
			})

			It("does not make any queries", func() {
				err := helper.Seed()
				Expect(err).NotTo(HaveOccurred())
				Expect(testLogger.Buffer()).To(Say("No preseeded databases specified, skipping seeding."))
				Expect(fakeSeeder.CreateDBIfNeededCallCount()).To(Equal(0))
				Expect(fakeSeeder.IsExistingUserCallCount()).To(Equal(0))
				Expect(fakeSeeder.CreateUserCallCount()).To(Equal(0))
				Expect(fakeSeeder.GrantUserPrivilegesCallCount()).To(Equal(0))
			})
		})
	})

	Describe("RunPostStartSQL", func() {
		It("runs the contents of the specified files", func() {
			mock.ExpectExec(fakeSupplementalQuery1).WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectExec(fakeSupplementalQuery2).WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

			err := helper.RunPostStartSQL()
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns an error when the database failes to execute a query", func() {
			err := helper.RunPostStartSQL()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("TestDatabaseCleanup", func() {
		It("removes 'test' and 'test_%' databases and related permissions, ignoring row close errors", func() {
			mock.ExpectExec(`DELETE FROM mysql.db WHERE Db IN\('test', 'test\\_%'\)`).WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectExec("FLUSH PRIVILEGES").WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectQuery("SHOW DATABASES LIKE 'test'").WillReturnRows(
				sqlmock.NewRows([]string{"DB"}).
					AddRow("test"),
			)
			mock.ExpectQuery(`SHOW DATABASES LIKE 'test\\_%'`).WillReturnRows(
				sqlmock.NewRows([]string{"DB"}).
					AddRow("test_2"),
			)

			mock.ExpectExec("DROP DATABASE IF EXISTS test").WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectExec("DROP DATABASE IF EXISTS test_2").WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

			Expect(helper.TestDatabaseCleanup()).To(Succeed())
		})

		It("errors if deleting permissions errors", func() {
			mock.ExpectExec(`DELETE FROM mysql.db WHERE Db IN\('test', 'test\\_%'\)`).WillReturnError(errors.New("foo"))

			err := helper.TestDatabaseCleanup()
			Expect(err).To(MatchError("foo"))
		})

		It("errors if flushing privileges errors", func() {
			mock.ExpectExec(`DELETE FROM mysql.db WHERE Db IN\('test', 'test\\_%'\)`).WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectExec("FLUSH PRIVILEGES").WillReturnError(errors.New("foo"))

			err := helper.TestDatabaseCleanup()
			Expect(err).To(MatchError("foo"))
		})

		It("errors if finding test database names errors", func() {
			mock.ExpectExec(`DELETE FROM mysql.db WHERE Db IN\('test', 'test\\_%'\)`).WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectExec("FLUSH PRIVILEGES").WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectQuery("SHOW DATABASES LIKE 'test'").WillReturnError(errors.New("foo"))

			err := helper.TestDatabaseCleanup()
			Expect(err).To(MatchError("foo"))
		})

		It("errors if finding test database names has a row error", func() {
			mock.ExpectExec(`DELETE FROM mysql.db WHERE Db IN\('test', 'test\\_%'\)`).WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectExec("FLUSH PRIVILEGES").WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectQuery("SHOW DATABASES LIKE 'test'").WillReturnRows(
				sqlmock.NewRows([]string{"DB"}).
					AddRow("test").
					RowError(0, errors.New("foo")),
			)

			err := helper.TestDatabaseCleanup()
			Expect(err).To(MatchError("foo"))
		})

		It("errors if scanning test database names errors", func() {
			mock.ExpectExec(`DELETE FROM mysql.db WHERE Db IN\('test', 'test\\_%'\)`).WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectExec("FLUSH PRIVILEGES").WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectQuery("SHOW DATABASES LIKE 'test'").WillReturnRows(
				sqlmock.NewRows([]string{"DB"}).
					AddRow(struct{}{}),
			)

			Expect(helper.TestDatabaseCleanup()).NotTo(Succeed())
		})

		It("errors if deleting a test database errors", func() {
			mock.ExpectExec(`DELETE FROM mysql.db WHERE Db IN\('test', 'test\\_%'\)`).WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectExec("FLUSH PRIVILEGES").WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectQuery("SHOW DATABASES LIKE 'test'").WillReturnRows(
				sqlmock.NewRows([]string{"DB"}).
					AddRow("test"),
			)
			mock.ExpectQuery(`SHOW DATABASES LIKE 'test\\_%'`).WillReturnRows(
				sqlmock.NewRows([]string{"DB"}).
					AddRow("test_2"),
			)

			mock.ExpectExec("DROP DATABASE IF EXISTS test").WillReturnError(errors.New("foo"))

			err := helper.TestDatabaseCleanup()
			Expect(err).To(MatchError("foo"))
		})

		It("errors if finding test_% database names errors", func() {
			mock.ExpectExec(`DELETE FROM mysql.db WHERE Db IN\('test', 'test\\_%'\)`).WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectExec("FLUSH PRIVILEGES").WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectQuery("SHOW DATABASES LIKE 'test'").WillReturnRows(
				sqlmock.NewRows([]string{"DB"}).
					AddRow("test"),
			)

			mock.ExpectQuery(`SHOW DATABASES LIKE 'test\\_%'`).WillReturnError(errors.New("foo"))

			err := helper.TestDatabaseCleanup()
			Expect(err).To(MatchError("foo"))
		})

		It("errors if finding test_% database names has a row error", func() {
			mock.ExpectExec(`DELETE FROM mysql.db WHERE Db IN\('test', 'test\\_%'\)`).WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectExec("FLUSH PRIVILEGES").WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectQuery("SHOW DATABASES LIKE 'test'").WillReturnRows(
				sqlmock.NewRows([]string{"DB"}).
					AddRow("test"),
			)

			mock.ExpectQuery(`SHOW DATABASES LIKE 'test\\_%'`).WillReturnRows(
				sqlmock.NewRows([]string{"DB"}).
					AddRow("test_2").
					RowError(0, errors.New("foo")),
			)

			err := helper.TestDatabaseCleanup()
			Expect(err).To(MatchError("foo"))
		})

		It("errors if scanning test_% database names errors", func() {
			mock.ExpectExec(`DELETE FROM mysql.db WHERE Db IN\('test', 'test\\_%'\)`).WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectExec("FLUSH PRIVILEGES").WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectQuery("SHOW DATABASES LIKE 'test'").WillReturnRows(
				sqlmock.NewRows([]string{"DB"}).
					AddRow("test"),
			)

			mock.ExpectQuery(`SHOW DATABASES LIKE 'test\\_%'`).WillReturnRows(
				sqlmock.NewRows([]string{"DB"}).
					AddRow(struct{}{}),
			)

			Expect(helper.TestDatabaseCleanup()).NotTo(Succeed())
		})

		It("errors if deleting a test_% database errors", func() {
			mock.ExpectExec(`DELETE FROM mysql.db WHERE Db IN\('test', 'test\\_%'\)`).WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectExec("FLUSH PRIVILEGES").WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectQuery("SHOW DATABASES LIKE 'test'").WillReturnRows(
				sqlmock.NewRows([]string{"DB"}).
					AddRow("test"),
			)
			mock.ExpectQuery(`SHOW DATABASES LIKE 'test\\_%'`).WillReturnRows(
				sqlmock.NewRows([]string{"DB"}).
					AddRow("test_2").
					AddRow("test_foo"),
			)

			mock.ExpectExec("DROP DATABASE IF EXISTS test").WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))
			mock.ExpectExec("DROP DATABASE IF EXISTS test_2").WillReturnError(errors.New("foo"))

			err := helper.TestDatabaseCleanup()
			Expect(err).To(MatchError("foo"))
		})
	})
})
