package mariadb_helper_test

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	os_fakes "github.com/cloudfoundry/mariadb_ctrl/os_helper/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gbytes"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("MariaDBHelper", func() {
	var (
		helper     *mariadb_helper.MariaDBHelper
		fakeOs     *os_fakes.FakeOsHelper
		testLogger lagertest.TestLogger
		logFile    string
		dbConfig   config.DBHelper
		fakeDB     *sql.DB
	)

	BeforeEach(func() {
		var err error
		fakeOs = new(os_fakes.FakeOsHelper)
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

		It("creates the specified databases", func() {

			for _, preseedDb := range dbConfig.PreseededDatabases {

				createDbExec := fmt.Sprintf(
					"CREATE DATABASE IF NOT EXISTS `%s`",
					preseedDb.DBName)
				sqlmock.ExpectExec(createDbExec).
					WithArgs().
					WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

				selectUsersQuery := fmt.Sprintf(
					"SELECT User FROM mysql\\.user WHERE User = '%s'",
					preseedDb.User)
				sqlmock.ExpectQuery(selectUsersQuery).
					WithArgs().
					WillReturnRows(sqlmock.NewRows([]string{"User"}))

				createUserExec := fmt.Sprintf(
					"CREATE USER `%s` IDENTIFIED BY '%s'",
					preseedDb.User,
					preseedDb.Password)
				sqlmock.ExpectExec(createUserExec).
					WithArgs().
					WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

				grantExec := fmt.Sprintf(
					"GRANT ALL ON `%s`\\.\\* TO `%s`",
					preseedDb.DBName,
					preseedDb.User)
				sqlmock.ExpectExec(grantExec).
					WithArgs().
					WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

			}

			sqlmock.ExpectExec("FLUSH PRIVILEGES").
				WithArgs().
				WillReturnResult(sqlmock.NewResult(lastInsertId, rowsAffected))

			helper.Seed()
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
			})
		})
	})
})
