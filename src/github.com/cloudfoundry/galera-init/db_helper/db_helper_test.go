package db_helper_test

import (
	"database/sql"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagertest"
	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"

	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/db_helper"
	"github.com/cloudfoundry/galera-init/db_helper/seeder"
	"github.com/cloudfoundry/galera-init/db_helper/seeder/seederfakes"
	"github.com/cloudfoundry/galera-init/os_helper/os_helperfakes"
)

var _ = Describe("GaleraDBHelper", func() {
	const (
		lastInsertId           = -1
		rowsAffected           = 1
		fakeSupplementalQuery1 = "some fake query"
		fakeSupplementalQuery2 = "another fake query"
	)

	var (
		helper     *db_helper.GaleraDBHelper
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
		testLogger = *lagertest.NewTestLogger("db_helper")

		fakeDB, mock, err = sqlmock.New()
		Expect(err).ToNot(HaveOccurred())
		db_helper.OpenDBConnection = func(*config.DBHelper) (*sql.DB, error) {
			return fakeDB, nil
		}
		db_helper.CloseDBConnection = func(*sql.DB) error {
			// fakeDB is closed in AfterEach to allow assertions against mock expectations
			return nil
		}

		db_helper.BuildSeeder = func(db *sql.DB, config config.PreseededDatabase, logger lager.Logger) seeder.Seeder {
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
		helper = db_helper.NewDBHelper(
			fakeOs,
			dbConfig,
			logFile,
			testLogger,
		)
	})

	AfterEach(func() {
		Expect(mock.ExpectationsWereMet()).To(Succeed())
	})

	Describe("StartMysqldForUpgrade", func() {
		BeforeEach(func() {
			fakeOs.StartCommandStub = func(logFile string, executable string, args ...string) (cmd *exec.Cmd, e error) {
				return exec.Command("stub"), nil
			}
		})

		It("start mysql in an upgrade mode and return an exec.Cmd value", func() {
			options := []string{
				"--defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf",
				"--wsrep-on=OFF",
				"--wsrep-desync=ON",
				"--wsrep-OSU-method=RSU",
				"--wsrep-provider=none",
				"--skip-networking",
			}
			cmd, err := helper.StartMysqldForUpgrade()
			Expect(err).NotTo(HaveOccurred())
			Expect(cmd).To(SatisfyAll(
				Not(BeNil()),
				BeAssignableToTypeOf(&exec.Cmd{}),
			))

			Expect(fakeOs.StartCommandCallCount()).To(Equal(1))
			logFile, executable, args := fakeOs.StartCommandArgsForCall(0)
			Expect(logFile).ToNot(BeEmpty())
			Expect(executable).To(Equal("mysqld"))
			Expect(args).To(Equal(options))
		})

		Context("when an error occurs while starting mysqld", func() {
			It("should return an error", func() {
				fakeOs.StartCommandStub = func(logfile string, command string, args ...string) (*exec.Cmd, error) {
					return nil, errors.New("starting somehow failed")
				}

				_, err := helper.StartMysqldForUpgrade()
				Expect(err).To(MatchError(`Error starting mysqld in stand-alone: starting somehow failed`))
			})
		})
	})

	Describe("StopMysqld", func() {
		It("calls the mysql daemon with the stop command", func() {
			fakeOs.RunCommandReturns("", nil)

			helper.StopMysqld()

			executable, args := fakeOs.RunCommandArgsForCall(0)
			Expect(executable).To(Equal("mysqladmin"))
			Expect(args).To(Equal([]string{"--defaults-file=/var/vcap/jobs/pxc-mysql/config/mylogin.cnf", "shutdown"}))
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
			Expect(args).To(Equal([]string{"--defaults-file=/var/vcap/jobs/pxc-mysql/config/mylogin.cnf", "status"}))
		})

		It("returns false if `mysql.server status` exits non-zero", func() {
			fakeOs.RunCommandReturns("", errors.New("error checking status"))

			isRunning := helper.IsProcessRunning()
			Expect(isRunning).To(BeFalse())

			Expect(fakeOs.RunCommandCallCount()).To(Equal(1))
			executable, args := fakeOs.RunCommandArgsForCall(0)
			Expect(executable).To(Equal("mysqladmin"))
			Expect(args).To(Equal([]string{"--defaults-file=/var/vcap/jobs/pxc-mysql/config/mylogin.cnf", "status"}))
		})
	})

	Describe("Upgrade", func() {
		It("calls the mysql upgrade script", func() {
			helper.Upgrade()
			Expect(fakeOs.RunCommandCallCount()).To(Equal(1))

			executable, args := fakeOs.RunCommandArgsForCall(0)
			Expect(executable).To(Equal(dbConfig.UpgradePath))
			Expect(args).To(Equal([]string{"--defaults-file=/var/vcap/jobs/pxc-mysql/config/mylogin.cnf"}))
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
})
