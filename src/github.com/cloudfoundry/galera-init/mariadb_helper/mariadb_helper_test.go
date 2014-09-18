package mariadb_helper_test

import (
	helper "."
	"errors"
	os_fakes "github.com/cloudfoundry/mariadb_ctrl/os_helper/fakes"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MariaDBHelper", func() {
	var mariadb_helper *helper.MariaDBHelper
	var fakeOs *os_fakes.FakeOsHelper
	var mysqlDaemonPath = "/mysqld"
	var logFile = "/log-file.log"
	var upgradeScriptPath = "/upgrade_script"
	var username = "user"
	var password = "password"

	BeforeEach(func() {
		fakeOs = new(os_fakes.FakeOsHelper)
		mariadb_helper = helper.NewMariaDBHelper(
			fakeOs,
			mysqlDaemonPath,
			logFile,
			false,
			upgradeScriptPath,
			username,
			password,
		)
	})

	Describe("Start", func() {

		Context("when the pgrep check shows that daemon is not already running", func() {
			BeforeEach(func() {
				fakeOs.RunCommandWithTimeoutStub = func(timeout int, logFileLocation string, executable string, args ...string) error {
					if args[0] == "pgrep" {
						return errors.New("did not find the daemon")
					} else {
						return nil
					}
				}
			})

			It("calls the mysql daemon with the command option", func() {
				mariadb_helper.StartMysqldInMode("bootstrap")
				Expect(fakeOs.RunCommandWithTimeoutCallCount()).To(Equal(2))

				timeout, logDestination, executable, args := fakeOs.RunCommandWithTimeoutArgsForCall(0)
				Expect(timeout).To(Equal(10))
				Expect(logDestination).To(Equal(logFile))
				Expect(executable).To(Equal("bash"))
				Expect(args).To(Equal([]string{"pgrep", "-f", mysqlDaemonPath}))

				timeout, logDestination, executable, args = fakeOs.RunCommandWithTimeoutArgsForCall(1)
				Expect(timeout).To(Equal(10))
				Expect(logDestination).To(Equal(logFile))
				Expect(executable).To(Equal("bash"))
				Expect(args).To(Equal([]string{mysqlDaemonPath, "bootstrap"}))
			})
		})

		Context("when the pgrep check shows that daemon is already running", func() {
			BeforeEach(func() {
				fakeOs.RunCommandWithTimeoutReturns(nil)
			})

			It("returns an error and does not call the start command", func() {
				err := mariadb_helper.StartMysqldInMode("bootstrap")
				Expect(err).To(HaveOccurred())
				Expect(fakeOs.RunCommandWithTimeoutCallCount()).To(Equal(1))

				timeout, logDestination, executable, args := fakeOs.RunCommandWithTimeoutArgsForCall(0)
				Expect(timeout).To(Equal(10))
				Expect(logDestination).To(Equal(logFile))
				Expect(executable).To(Equal("bash"))
				Expect(args).To(Equal([]string{"pgrep", "-f", mysqlDaemonPath}))
			})
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
			mariadb_helper.StopMysqld()
			Expect(fakeOs.RunCommandWithTimeoutCallCount()).To(Equal(1))

			timeout, logDestination, executable, args := fakeOs.RunCommandWithTimeoutArgsForCall(0)
			Expect(timeout).To(Equal(10))
			Expect(logDestination).To(Equal(logFile))
			Expect(executable).To(Equal("bash"))
			Expect(args).To(Equal([]string{mysqlDaemonPath, "stop"}))
		})

		Context("when an error occurs", func() {
			BeforeEach(func() {
				fakeOs.RunCommandWithTimeoutReturns(errors.New("some error"))
			})

			It("returns the error", func() {
				err := mariadb_helper.StopMysqld()
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
