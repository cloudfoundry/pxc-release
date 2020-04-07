package os_helper_test

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"

	. "github.com/cloudfoundry/galera-init/os_helper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("OsHelper", func() {
	var (
		helper *OsHelperImpl
	)

	BeforeEach(func() {
		helper = NewImpl()
	})

	Describe("StartCommand", func() {
		var (
			tempDir     string
			logFilePath string
		)

		BeforeEach(func() {
			var err error
			tempDir, err = ioutil.TempDir(os.TempDir(), "start_command_")
			Expect(err).NotTo(HaveOccurred())

			logFilePath = filepath.Join(tempDir, "command.log")
		})

		AfterEach(func() {
			if tempDir != "" {
				_ = os.RemoveAll(tempDir)
			}
		})

		It("Runs a command", func() {
			cmd, err := helper.StartCommand(logFilePath, "echo", "-n", "some argument")
			Expect(err).NotTo(HaveOccurred())
			Expect(cmd.Wait()).To(Succeed())

			contents, err := ioutil.ReadFile(logFilePath)
			Expect(err).NotTo(HaveOccurred())

			Expect(string(contents)).To(Equal("some argument"))
		})

		It("has the right permissions for the logfile", func() {
			cmd, err := helper.StartCommand(logFilePath, "echo", "-n", "some argument")
			Expect(err).NotTo(HaveOccurred())
			Expect(cmd.Wait()).To(Succeed())

			fileInfo,_ := os.Stat(logFilePath)
			Expect(fileInfo.Mode().String()).To(Equal("-rw-r--r--"))
		})

		When("an invalid logFileName is requested", func() {
			It("returns an error", func() {
				badPath := filepath.Join(tempDir, "log.directory")
				Expect(os.Mkdir(badPath, 0750)).To(Succeed())

				_, err := helper.StartCommand(badPath, "echo", "-n", "some argument")
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).
					To(
						ContainSubstring(`error logging output for command "echo"`),
					)
			})
		})

		When("an invalid executable path is requested", func() {
			It("returns an error", func() {
				badExecutable := filepath.Join(tempDir, "command-does-not-exist")

				_, err := helper.StartCommand(logFilePath, badExecutable)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).
					To(MatchRegexp(`error starting .*/command-does-not-exist.* no such file or directory`))
			})
		})
	})

	Describe("WaitForCommand", func() {

		Context("When command is bad", func() {
			It("Sends an error to a channel when the process exits", func() {
				h := OsHelperImpl{}
				cmd := exec.Command("non-existent-command")
				cmd.Start()
				ch := h.WaitForCommand(cmd)
				err := <-ch
				Expect(err).NotTo(BeNil())
			})
		})

		Context("When command is good", func() {
			It("Sends nil to a channel when the process exits", func() {
				h := OsHelperImpl{}
				cmd := exec.Command("ls")
				cmd.Start()
				ch := h.WaitForCommand(cmd)
				err := <-ch
				Expect(err).To(BeNil())
			})
		})
	})

	Describe("KillCommand", func() {
		var helper OsHelperImpl

		It("sends the provided signal to the provided cmd process", func() {
			cmd := exec.Command("sleep", "8")
			Expect(cmd.Start()).To(Succeed())
			err := helper.KillCommand(cmd, syscall.SIGKILL)
			Expect(err).NotTo(HaveOccurred())

			err = cmd.Wait()
			Expect(err).To(MatchError(`signal: killed`))
		})

		It("returns a useful error when unable to signal the process", func() {
			cmd := exec.Command("sleep", "0")
			Expect(cmd.Run()).To(Succeed())
			err := helper.KillCommand(cmd, syscall.SIGKILL)
			Expect(err).To(MatchError("unable-to-kill-process: os: process already finished"))
		})

		Context("when the process hasn't started", func() {
			It("errors nicely when the cmd has no process", func() {
				cmd := exec.Command("sleep", "8")
				err := helper.KillCommand(cmd, syscall.SIGKILL)
				Expect(err).To(MatchError("process-was-not-started"))
			})
			It("errors nicely when the cmd is nil", func() {
				var cmd *exec.Cmd

				err := helper.KillCommand(cmd, syscall.SIGKILL)
				Expect(err).To(MatchError("process-was-not-started"))
			})
		})

	})
})
