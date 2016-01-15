package mariadb_helper_test

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	s "github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/seeder"
	seeder_fakes "github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/seeder/fakes"
	os_fakes "github.com/cloudfoundry/mariadb_ctrl/os_helper/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("MariaDBHelper", func() {
	var (
		helper     *mariadb_helper.MariaDBHelper
		fakeOs     *os_fakes.FakeOsHelper
		fakeSeeder *seeder_fakes.FakeSeeder
		testLogger lagertest.TestLogger
		logFile    string
		dbConfig   config.DBHelper
		fakeDB     *sql.DB
	)

	BeforeEach(func() {
		var err error
		fakeOs = new(os_fakes.FakeOsHelper)
		fakeSeeder = new(seeder_fakes.FakeSeeder)
		testLogger = *lagertest.NewTestLogger("mariadb_helper")

		fakeDB, err = sqlmock.New()
		Expect(err).ToNot(HaveOccurred())
		mariadb_helper.OpenDBConnection = func(config.DBHelper) (*sql.DB, error) {
			return fakeDB, nil
		}
		mariadb_helper.CloseDBConnection = func(*sql.DB) error {
			// fakeDB is closed in AfterEach to allow assertions against mock expectations
			return nil
		}

		mariadb_helper.BuildSeeder = func(db *sql.DB, config config.PreseededDatabase, logger lager.Logger) s.Seeder {
			return fakeSeeder
		}

		logFile = "/log-file.log"
		dbConfig = config.DBHelper{
			DaemonPath:  "/mysqld",
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
		err := fakeDB.Close()
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Start", func() {

		It("calls the mysql daemon with the command option", func() {
			helper.StartMysqldInMode("bootstrap")
			Expect(fakeOs.RunCommandWithTimeoutCallCount()).To(Equal(1))

			timeout, logDestination, executable, args := fakeOs.RunCommandWithTimeoutArgsForCall(0)
			Expect(timeout).To(Equal(10))
			Expect(logDestination).To(Equal(logFile))
			Expect(executable).To(Equal("bash"))
			Expect(args).To(Equal([]string{dbConfig.DaemonPath, "bootstrap"}))
		})

		Context("when an error occurs", func() {
			BeforeEach(func() {
				fakeOs.RunCommandWithTimeoutReturns(errors.New("some error"))
			})

			It("returns the error", func() {
				err := helper.StartMysqldInMode("bootstrap")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Stop", func() {
		It("calls the mysql daemon with the stop command", func() {
			helper.StopStandaloneMysql()
			Expect(fakeOs.RunCommandWithTimeoutCallCount()).To(Equal(1))

			timeout, logDestination, executable, args := fakeOs.RunCommandWithTimeoutArgsForCall(0)
			Expect(timeout).To(Equal(10))
			Expect(logDestination).To(Equal(logFile))
			Expect(executable).To(Equal("bash"))
			Expect(args).To(Equal([]string{dbConfig.DaemonPath, mariadb_helper.StopStandaloneCommand}))
		})

		Context("when an error occurs", func() {
			BeforeEach(func() {
				fakeOs.RunCommandWithTimeoutReturns(errors.New("some error"))
			})

			It("returns the error", func() {
				err := helper.StopStandaloneMysql()
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("IsProcessRunning", func() {
		It("returns true if `mysql.server status` exits zero", func() {
			fakeOs.RunCommandReturns(" * MySQL running (2391)", nil)

			isRunning := helper.IsProcessRunning()
			Expect(isRunning).To(BeTrue())
		})

		It("returns false if `mysql.server status` exits non-zero", func() {
			fakeOs.RunCommandWithTimeoutStub = func(_ int, _, _ string, args ...string) error {
				mode := args[1]
				if mode == mariadb_helper.StatusCommand {
					return errors.New("not running error")
				} else {
					return nil
				}
			}

			isRunning := helper.IsProcessRunning()
			Expect(isRunning).To(BeFalse())
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

	Describe("Seed", func() {

		const lastInsertId = -1
		const rowsAffected = 1

		Context("when there are pre-seeded databases", func() {

			Context("if the users already exist", func() {
				BeforeEach(func() {
					fakeSeeder.IsExistingUserReturns(true, nil)
				})

				It("creates the specified databases without creating users", func() {
					sqlmock.ExpectExec("FLUSH PRIVILEGES").
						WithArgs().
						WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

					helper.Seed()

					Expect(fakeSeeder.CreateDBIfNeededCallCount()).To(Equal(2))
					Expect(fakeSeeder.IsExistingUserCallCount()).To(Equal(2))
					Expect(fakeSeeder.CreateUserForDBCallCount()).To(Equal(0))
					Expect(fakeSeeder.GrantUserAllPrivilegesCallCount()).To(Equal(2))
				})
			})

			Context("if the users do not exist", func() {
				BeforeEach(func() {
					fakeSeeder.IsExistingUserReturns(false, nil)
				})

				It("creates the specified databases and creates users", func() {
					sqlmock.ExpectExec("FLUSH PRIVILEGES").
						WithArgs().
						WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

					helper.Seed()

					Expect(fakeSeeder.CreateDBIfNeededCallCount()).To(Equal(2))
					Expect(fakeSeeder.IsExistingUserCallCount()).To(Equal(2))
					Expect(fakeSeeder.CreateUserForDBCallCount()).To(Equal(2))
					Expect(fakeSeeder.GrantUserAllPrivilegesCallCount()).To(Equal(2))
				})
			})

			Context("if a seeder function call returns an error", func() {
				It("returns an error back", func() {

					fakeSeeder.CreateDBIfNeededReturns(errors.New("Error"))
					err := helper.Seed()
					Expect(err).To(HaveOccurred())

					fakeSeeder.IsExistingUserReturns(false, errors.New("Error"))
					err = helper.Seed()
					Expect(err).To(HaveOccurred())

					fakeSeeder.CreateUserForDBReturns(errors.New("Error"))
					err = helper.Seed()
					Expect(err).To(HaveOccurred())

					fakeSeeder.GrantUserAllPrivilegesReturns(errors.New("Error"))
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
				//expect no queries or execs
				err := helper.Seed()
				Expect(err).NotTo(HaveOccurred())
				Expect(testLogger.Buffer()).To(Say("No preseeded databases specified, skipping seeding."))
				Expect(fakeSeeder.CreateDBIfNeededCallCount()).To(Equal(0))
				Expect(fakeSeeder.IsExistingUserCallCount()).To(Equal(0))
				Expect(fakeSeeder.CreateUserForDBCallCount()).To(Equal(0))
				Expect(fakeSeeder.GrantUserAllPrivilegesCallCount()).To(Equal(0))
			})
		})
	})

	Describe("CreateSuperROUser", func() {
		It("creates a read-only user successfully", func() {
			err := helper.CreateSuperROUser()
			Expect(fakeSeeder.CreateUserCallCount()).To(Equal(1))
			//TODO: Check args for above call
			Expect(fakeSeeder.GrantUserSuperROPrivilegesCallCount()).To(Equal(1))
			//TODO: Check args for above call
			Expect(err).NotTo(HaveOccurred())
		})

		Context("if a seeder function call returns an error", func() {
			It("returns an error back", func() {

				fakeSeeder.CreateUserReturns(errors.New("Error"))
				err := helper.CreateSuperROUser()
				Expect(err).To(HaveOccurred())

				fakeSeeder.GrantUserSuperROPrivilegesReturns(errors.New("Error"))
				err = helper.CreateSuperROUser()
				Expect(err).To(HaveOccurred())
			})
		})
	})

})
