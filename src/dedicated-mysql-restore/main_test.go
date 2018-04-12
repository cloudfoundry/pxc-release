package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"dedicated-mysql-restore/cmdopts"
	"dedicated-mysql-restore/executable/executablefakes"
)

type MockLogger struct {
	*log.Logger
	buffer *gbytes.Buffer
}

func NewMockLogger() *MockLogger {
	return &MockLogger{
		log.New(GinkgoWriter, "", log.LstdFlags),
		gbytes.NewBuffer(),
	}
}

func (mockLogger *MockLogger) Output(calldepth int, s string) error {
	_, err := fmt.Fprint(mockLogger.buffer, s)
	return err
}

func (mockLogger *MockLogger) Fatal(v ...interface{}) {
	err := mockLogger.Output(2, fmt.Sprint(v...))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	panic("Fatal")
}

func (mockLogger *MockLogger) Fatalf(format string, v ...interface{}) {
	err := mockLogger.Output(2, fmt.Sprintf(format, v...))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	panic("Fatalf")
}

var _ = Describe("Main", func() {
	var (
		err        error
		mockLogger *MockLogger
		oldArgs    []string
	)
	BeforeEach(func() {
		mockLogger = NewMockLogger()
		logger = mockLogger
		oldArgs = append(oldArgs, os.Args...)
		os.Args = append(os.Args,
			"--encryption-key", "secret",
			"--restore-file", "/tmp/bup.tar.gpg",
		)
	})

	It("Returns an error if the required arguments are not passed", func() {
		_, err = cmdopts.ParseArgs(oldArgs)
		Expect(err).To(HaveOccurred())
	})

	Context("When user is not root", func() {
		AfterEach(func() {
			rootUID = os.Geteuid()
		})
		It("Fails if user is not root", func() {
			rootUID = os.Geteuid() + 1
			Expect(main).To(Panic())
			Eventually(mockLogger.buffer).Should(gbytes.Say("Restore utility requires root privileges. Please run as root user."))
		})
	})

	Context("When the user has the correct privileges", func() {
		It("Fails if the restore file does not exist", func() {
			Expect(main).To(Panic())
			Eventually(mockLogger.buffer).Should(gbytes.Say("Failed to open restore file '/tmp/bup.tar.gpg': open /tmp/bup.tar.gpg: no such file or directory"))
		})
	})

	Context("When an encrypted backup file is supplied", func() {
		It("Fails if the encrypted GPG file is corrupt", func() {
			err = ioutil.WriteFile("/tmp/bup.tar.gpg", []byte("foobar"), 0600)
			Expect(err).NotTo(HaveOccurred())
			defer func() {
				err = os.Remove("/tmp/bup.tar.gpg")
				Expect(err).NotTo(HaveOccurred())
			}()
			Expect(main).To(Panic())
			Eventually(mockLogger.buffer).Should(gbytes.Say("Failed to open gpg archive '/tmp/bup.tar.gpg': openpgp: invalid data: tag byte does not have MSB set"))
		})

		It("Fails if the GPG password is wrong", func() {
			os.Args = append(oldArgs,
				"--encryption-key", "not-secret",
				"--restore-file", "unpack/secret.gpg",
			)
			Expect(main).To(Panic())
			Eventually(mockLogger.buffer).Should(gbytes.Say("Failed to open gpg archive 'unpack/secret.gpg': failed to decrypt with encryption key"))
		})

	})

	Context("stopMysqlMonitProcesses", func() {
		var (
			mock *executablefakes.FakeExecutable
		)
		BeforeEach(func() {
			mock = new(executablefakes.FakeExecutable)
			execer = mock
		})

		It("runs monit stop", func() {
			err = stopMysqlMonitProcesses()
			Expect(err).NotTo(HaveOccurred())
			Expect(mock.RunCallCount()).To(Equal(2))
			Expect(mock.RunArgsForCall(0).Args).To(Equal([]string{monitPath, "unmonitor", "mysql"}))
			Expect(mock.RunArgsForCall(1).Args).To(Equal([]string{"/var/vcap/jobs/mysql/bin/mysql_ctl", "stop"}))
		})

		It("fails if the monit unmonitor command fails", func() {
			mock.RunReturns(fmt.Errorf("monit unmonitor failed"))
			err = stopMysqlMonitProcesses()
			Expect(err).To(MatchError("monit unmonitor failed"))
			Expect(mock.RunCallCount()).To(Equal(1))
			Expect(mock.RunArgsForCall(0).Args).To(Equal([]string{monitPath, "unmonitor", "mysql"}))
		})

		It("fails if mysql_ctl stop fails", func() {
			mock.RunReturnsOnCall(1, fmt.Errorf("mysql_ctl stop failed"))
			err = stopMysqlMonitProcesses()
			Expect(err).To(MatchError("mysql_ctl stop failed"))
			Expect(mock.RunCallCount()).To(Equal(2))
			Expect(mock.RunArgsForCall(0).Args).To(Equal([]string{monitPath, "unmonitor", "mysql"}))
			Expect(mock.RunArgsForCall(1).Args).To(Equal([]string{"/var/vcap/jobs/mysql/bin/mysql_ctl", "stop"}))
		})
	})

	Context("When unpacking a backup", func() {
		It("Returns an error message if the tar archive is corrupt", func() {
			mock := new(executablefakes.FakeExecutable)
			execer = mock
			os.Args = append(os.Args,
				"--encryption-key", "secret",
				"--restore-file", "unpack/secret.gpg",
			)
			rootUID = os.Geteuid()

			dir, err := ioutil.TempDir("", "main_test_")
			Expect(err).NotTo(HaveOccurred())
			mysqlDataDir = dir

			defer func() {
				err = os.RemoveAll(dir)
				Expect(err).NotTo(HaveOccurred())
			}()

			Expect(main).To(Panic())
			Eventually(mockLogger.buffer).Should(gbytes.Say(`Unpacking mysql backup failed: exit status \d+`))
		})
	})
})
