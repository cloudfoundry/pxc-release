package db_helper_test

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"os/exec"

	"code.cloudfoundry.org/lager/v3/lagertest"
	"github.com/DATA-DOG/go-sqlmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/format"

	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/db_helper"
	"github.com/cloudfoundry/galera-init/os_helper/os_helperfakes"
)

var _ = Describe("GaleraDBHelper", func() {
	const (
		fakeSupplementalQuery1 = "some fake query"
		fakeSupplementalQuery2 = "another fake query"
	)

	var (
		helper     *db_helper.GaleraDBHelper
		fakeOs     *os_helperfakes.FakeOsHelper
		testLogger lagertest.TestLogger
		logFile    string
		dbConfig   *config.DBHelper
		fakeDB     *sql.DB
		mock       sqlmock.Sqlmock
	)

	BeforeEach(func() {
		var err error
		fakeOs = new(os_helperfakes.FakeOsHelper)
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

		logFile = "/log-file.log"

		sqlFile1, _ := os.CreateTemp(os.TempDir(), "fake_sql_file")
		defer func(sqlFile1 *os.File) {
			_ = sqlFile1.Close()
		}(sqlFile1)
		sqlFile2, _ := os.CreateTemp(os.TempDir(), "fake_sql_file")
		defer func(sqlFile2 *os.File) {
			_ = sqlFile2.Close()
		}(sqlFile2)

		Expect(os.WriteFile(sqlFile1.Name(), []byte(fakeSupplementalQuery1), 755)).To(Succeed())
		Expect(os.WriteFile(sqlFile2.Name(), []byte(fakeSupplementalQuery2), 755)).To(Succeed())

		dbConfig = &config.DBHelper{
			User:     "user",
			Password: "password",
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

	Describe("StartMysqldInJoin", func() {
		It("calls mysqld with the right args", func() {
			fakeOs.RunCommandReturns("", nil)
			helper.StartMysqldInJoin()

			_, executable, args := fakeOs.StartCommandArgsForCall(0)
			Expect(executable).To(Equal("mysqld"))
			Expect(args).To(Equal([]string{"--defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf", "--defaults-group-suffix=_plugin"}))

		})
	})

	Describe("StartMysqldInBootstrap", func() {
		It("calls mysqld with the right args", func() {
			fakeOs.RunCommandReturns("", nil)
			helper.StartMysqldInBootstrap()

			_, executable, args := fakeOs.StartCommandArgsForCall(0)
			Expect(executable).To(Equal("mysqld"))
			Expect(args).To(Equal([]string{"--defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf", "--defaults-group-suffix=_plugin", "--wsrep-new-cluster"}))

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

	Describe("IsDatabaseReachable", func() {
		galeraReadyQuery := `SHOW GLOBAL STATUS LIKE 'wsrep\\_local\\_state\\_comment'`
		wsrepProviderQuery := `SHOW GLOBAL VARIABLES LIKE 'wsrep\\_provider'`

		Describe("when the ready check fails", func() {
			BeforeEach(func() {
				mock.ExpectQuery(wsrepProviderQuery).
					WillReturnError(fmt.Errorf("some error"))

			})

			It("returns false", func() {
				Expect(helper.IsDatabaseReachable()).To(BeFalse())
			})
		})

		Describe("when galera is enabled", func() {
			BeforeEach(func() {
				mock.ExpectQuery(wsrepProviderQuery).
					WillReturnRows(sqlmock.NewRows([]string{"Variable_name", "Value"}).
						AddRow("wsrep_provider", "something other than none"))
			})

			Describe("when the galera check returns Synced", func() {
				BeforeEach(func() {

					mock.ExpectQuery(galeraReadyQuery).
						WillReturnRows(sqlmock.NewRows([]string{"Variable_name", "Value"}).
							AddRow("wsrep_local_state_comment", "Synced"))
				})

				It("returns true", func() {
					Expect(helper.IsDatabaseReachable()).To(BeTrue())
				})
			})

			Describe("when the galera check returns other than Synced", func() {
				BeforeEach(func() {
					mock.ExpectQuery(galeraReadyQuery).
						WillReturnRows(sqlmock.NewRows([]string{"Variable_name", "Value"}).
							AddRow("wsrep_local_state_comment", "Totally not synced bruh"))
				})

				It("returns false", func() {
					Expect(helper.IsDatabaseReachable()).To(BeFalse())
				})
			})
		})

		Describe("when galera is disabled", func() {

			Describe("when wsrep_provider is not even specified", func() {
				It("returns true if it can query the db", func() {
					mock.ExpectQuery(wsrepProviderQuery).
						WillReturnError(sql.ErrNoRows)

					Expect(helper.IsDatabaseReachable()).To(BeTrue())
				})
			})

			It("returns true", func() {
				mock.ExpectQuery(wsrepProviderQuery).
					WillReturnRows(sqlmock.NewRows([]string{"Variable_name", "Value"}).
						AddRow("wsrep_provider", "none"))
				Expect(helper.IsDatabaseReachable()).To(BeTrue())
			})
		})

		Describe("when db connection can't be opened", func() {
			BeforeEach(func() {
				db_helper.OpenDBConnection = func(*config.DBHelper) (*sql.DB, error) {
					return nil, fmt.Errorf("whoops")
				}
			})

			It("returns false", func() {
				Expect(helper.IsDatabaseReachable()).To(BeFalse())
			})
		})

	})

	Describe("FormatDSN", func() {
		Context("When SkipBinlog is enabled", func() {
			It("formats a connection string with binlogging disabled", func() {
				config := config.DBHelper{
					Password:   "some-password",
					SkipBinlog: true,
					Socket:     "/some/socket/path.sock",
					User:       "some-user",
				}
				format.TruncatedDiff = false
				Expect(db_helper.FormatDSN(config)).To(Equal(`some-user:some-password@unix(/some/socket/path.sock)/?sql_log_bin=off`))
			})
		})

		Context("When SkipBinlog is not enabled", func() {
			It("formats a connection string without binlogging disabled", func() {
				config := config.DBHelper{
					Password: "some-password",
					Socket:   "/some/socket/path.sock",
					User:     "some-user",
				}
				format.TruncatedDiff = false
				Expect(db_helper.FormatDSN(config)).To(Equal(`some-user:some-password@unix(/some/socket/path.sock)/`))
			})
		})
	})

	Describe("StartMysqldInBootstrap", func() {
		It("starts mysqld with the --wsrep-new-cluster option", func() {
			fakeOs.StartCommandStub = func(_ string, executable string, args ...string) (*exec.Cmd, error) {
				return exec.Command(executable, args...), nil
			}

			cmd, err := helper.StartMysqldInBootstrap()
			Expect(err).NotTo(HaveOccurred())
			Expect(cmd).ToNot(BeNil())

			Expect(fakeOs.StartCommandCallCount()).To(Equal(1))

			logPath, executable, args := fakeOs.StartCommandArgsForCall(0)
			Expect(logPath).To(Equal("/log-file.log"))
			Expect(executable).To(Equal("mysqld"))
			Expect(args).To(Equal([]string{
				"--defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf",
				"--defaults-group-suffix=_plugin",
				"--wsrep-new-cluster",
			}))

		})

		Context("when an error occurs", func() {
			BeforeEach(func() {
				fakeOs.StartCommandReturns(nil, fmt.Errorf("injected StartCommand error"))
			})
			It("returns an error", func() {
				cmd, err := helper.StartMysqldInBootstrap()
				Expect(err).To(MatchError(`injected StartCommand error`))
				Expect(cmd).To(BeNil())
			})
		})

	})

	Describe("StartMysqldInJoin", func() {
		It("starts mysqld", func() {
			fakeOs.StartCommandStub = func(_ string, executable string, args ...string) (*exec.Cmd, error) {
				return exec.Command(executable, args...), nil
			}

			cmd, err := helper.StartMysqldInJoin()
			Expect(err).NotTo(HaveOccurred())
			Expect(cmd).ToNot(BeNil())

			Expect(fakeOs.StartCommandCallCount()).To(Equal(1))

			logPath, executable, args := fakeOs.StartCommandArgsForCall(0)
			Expect(logPath).To(Equal("/log-file.log"))
			Expect(executable).To(Equal("mysqld"))
			Expect(args).To(Equal([]string{
				"--defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf",
				"--defaults-group-suffix=_plugin",
			}))

		})

		Context("when an error occurs", func() {
			BeforeEach(func() {
				fakeOs.StartCommandReturns(nil, fmt.Errorf("injected StartCommand error"))
			})
			It("returns an error", func() {
				cmd, err := helper.StartMysqldInJoin()
				Expect(err).To(MatchError(`injected StartCommand error`))
				Expect(cmd).To(BeNil())
			})
		})

	})
})
