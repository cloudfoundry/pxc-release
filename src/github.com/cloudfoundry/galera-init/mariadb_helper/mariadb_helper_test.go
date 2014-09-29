package mariadb_helper_test

import (
	helper "."
	"errors"
	logger_fakes "github.com/cloudfoundry/mariadb_ctrl/logger/fakes"
	os_fakes "github.com/cloudfoundry/mariadb_ctrl/os_helper/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MariaDBHelper", func() {
	var mariadb_helper *helper.MariaDBHelper
	var fakeOs *os_fakes.FakeOsHelper
	var fakeLogger *logger_fakes.FakeLogger

	mysqlDaemonPath := "/mysqld"
	mysqlClientPath := "/mysqlClientPath"
	logFile := "/log-file.log"
	upgradeScriptPath := "/upgrade_script"
	showDatabasesScriptPath := "/showDatabasesScriptPath"
	username := "user"
	password := "password"

	BeforeEach(func() {
		fakeOs = new(os_fakes.FakeOsHelper)
		fakeLogger = new(logger_fakes.FakeLogger)

		mariadb_helper = helper.NewMariaDBHelper(
			fakeOs,
			mysqlDaemonPath,
			mysqlClientPath,
			logFile,
			fakeLogger,
			upgradeScriptPath,
			showDatabasesScriptPath,
			username,
			password,
		)
	})

	Describe("Start", func() {

		It("calls the mysql daemon with the command option", func() {
			mariadb_helper.StartMysqldInMode("bootstrap")
			Expect(fakeOs.RunCommandWithTimeoutCallCount()).To(Equal(1))

			timeout, logDestination, executable, args := fakeOs.RunCommandWithTimeoutArgsForCall(0)
			Expect(timeout).To(Equal(10))
			Expect(logDestination).To(Equal(logFile))
			Expect(executable).To(Equal("bash"))
			Expect(args).To(Equal([]string{mysqlDaemonPath, "bootstrap"}))
		})

		Context("when an error occurs", func() {
			BeforeEach(func() {
				fakeOs.RunCommandWithTimeoutReturns(errors.New("some error"))
			})

			It("returns the error", func() {
				err := mariadb_helper.StartMysqldInMode("bootstrap")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Stop", func() {
		It("calls the mysql daemon with the stop command", func() {
			mariadb_helper.StopStandaloneMysql()
			Expect(fakeOs.RunCommandWithTimeoutCallCount()).To(Equal(1))

			timeout, logDestination, executable, args := fakeOs.RunCommandWithTimeoutArgsForCall(0)
			Expect(timeout).To(Equal(10))
			Expect(logDestination).To(Equal(logFile))
			Expect(executable).To(Equal("bash"))
			Expect(args).To(Equal([]string{mysqlDaemonPath, helper.STOP_STANDALONE_COMMAND}))
		})

		Context("when an error occurs", func() {
			BeforeEach(func() {
				fakeOs.RunCommandWithTimeoutReturns(errors.New("some error"))
			})

			It("returns the error", func() {
				err := mariadb_helper.StopStandaloneMysql()
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Upgrade", func() {
		It("calls the mysql upgrade script", func() {
			mariadb_helper.Upgrade()
			Expect(fakeOs.RunCommandCallCount()).To(Equal(1))

			executable, args := fakeOs.RunCommandArgsForCall(0)
			Expect(executable).To(Equal("bash"))
			Expect(args).To(Equal([]string{upgradeScriptPath, username, password, logFile}))
		})

		It("returns the output and error", func() {
			fakeOs.RunCommandReturns("some output", errors.New("some error"))
			output, err := mariadb_helper.Upgrade()
			Expect(output).To(Equal("some output"))
			Expect(err.Error()).To(Equal("some error"))
		})
	})
})
