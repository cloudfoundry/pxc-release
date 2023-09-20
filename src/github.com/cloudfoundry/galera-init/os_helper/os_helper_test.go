package os_helper_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	. "github.com/cloudfoundry/galera-init/os_helper"
	"github.com/google/uuid"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("OsHelper", func() {
	var (
		helper *OsHelperImpl
	)

	BeforeEach(func() {
		helper = NewImpl()
	})

	Describe("RunCommand", func() {
		It("Runs a command", func() {
			output, err := helper.RunCommand("echo", "-n", "some data")
			Expect(err).NotTo(HaveOccurred())

			Expect(output).To(Equal("some data"))
		})

		When("running the command fails", func() {
			It("returns an error and still returns the output", func() {
				output, err := helper.RunCommand("bash", "-c", "echo >&2 -n some data && false")
				Expect(err).To(HaveOccurred())

				Expect(output).To(Equal("some data"))
			})
		})
	})

	Describe("StartCommand", func() {
		var (
			tempDir     string
			logFilePath string
		)

		BeforeEach(func() {
			var err error
			tempDir, err = os.MkdirTemp("", "start_command_")
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

			contents, err := os.ReadFile(logFilePath)
			Expect(err).NotTo(HaveOccurred())

			Expect(string(contents)).To(Equal("some argument"))
		})

		It("has the right permissions for the logfile", func() {
			cmd, err := helper.StartCommand(logFilePath, "echo", "-n", "some argument")
			Expect(err).NotTo(HaveOccurred())
			Expect(cmd.Wait()).To(Succeed())

			fileInfo, _ := os.Stat(logFilePath)
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
				Expect(cmd.Start()).Error().To(HaveOccurred())
				ch := h.WaitForCommand(cmd)
				err := <-ch
				Expect(err).NotTo(BeNil())
			})
		})

		Context("When command is good", func() {
			It("Sends nil to a channel when the process exits", func() {
				h := OsHelperImpl{}
				cmd := exec.Command("ls")
				Expect(cmd.Start()).Error().NotTo(HaveOccurred())
				ch := h.WaitForCommand(cmd)
				err := <-ch
				Expect(err).To(BeNil())
			})
		})
	})

	Describe("FileExists", func() {
		When("a file exists", func() {
			var tempDir string

			BeforeEach(func() {
				var err error
				tempDir, err = os.MkdirTemp("", "start_command_")
				Expect(err).NotTo(HaveOccurred())
				Expect(os.WriteFile(tempDir+"/foo", nil, 0644)).Error().NotTo(HaveOccurred())
			})

			It("returns true", func() {
				Expect(helper.FileExists(tempDir + "/foo")).To(BeTrue())
			})
		})
		When("a file does not exist", func() {
			BeforeEach(func() {
			})

			It("returns true", func() {
				Expect(helper.FileExists("/tmp/" + uuid.NewString())).To(BeFalse())
			})
		})
	})

	Describe("ReadFile", func() {
		var tempDir string

		BeforeEach(func() {
			var err error
			tempDir, err = os.MkdirTemp("", "start_command_")
			Expect(err).NotTo(HaveOccurred())
			Expect(os.WriteFile(tempDir+"/foo", []byte("some fancy data: \U0001f37f"), 0644)).Error().NotTo(HaveOccurred())
		})

		It("reads the contents of a file and returns the string content", func() {
			content, err := helper.ReadFile(tempDir + "/foo")
			Expect(err).NotTo(HaveOccurred())

			Expect(content).To(Equal("some fancy data: 🍿"))
			Expect(content).To(BeAssignableToTypeOf(string("")))
		})

		When("reading a file fails", func() {
			It("returns an error", func() {
				_, err := helper.ReadFile("/tmp/" + uuid.NewString())
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("WriteStringToFile", func() {
		var tempDir string

		BeforeEach(func() {
			var err error
			tempDir, err = os.MkdirTemp("", "start_command_")
			Expect(err).NotTo(HaveOccurred())
		})

		It("writes a string to a file", func() {
			err := helper.WriteStringToFile(tempDir+"/foo", "some fancy string")
			Expect(err).NotTo(HaveOccurred())

			contents, err := os.ReadFile(tempDir + "/foo")
			Expect(err).NotTo(HaveOccurred())
			Expect(string(contents)).To(Equal("some fancy string"))
		})

		When("writing to a file fails", func() {
			It("returns an error", func() {
				err := helper.WriteStringToFile(tempDir+"/invalid-directory/"+uuid.NewString(), "anything")
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Describe("Sleep", func() {
		It("sleeps for the specified duration", func() {
			now := time.Now()
			helper.Sleep(time.Second)

			// Expect approximately one second +- some noise
			// Picked "noise" as 250ms, which should be sufficient to avoid excessive flakes in CI
			Expect(time.Since(now)).To(BeNumerically("~", time.Second, 250*time.Millisecond))
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
