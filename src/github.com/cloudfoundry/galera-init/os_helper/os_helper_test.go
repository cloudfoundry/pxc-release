package os_helper_test

import (
	"os"
	"os/exec"
	"strings"

	. "github.com/cloudfoundry/galera-init/os_helper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("OsHelper", func() {
	Describe("StartCommand", func() {
		Context("when APPEND_TO_PATH is set", func() {
			BeforeEach(func() {
				os.Setenv("APPEND_TO_PATH", "/other/packages:/and/more/dirs")
			})

			It("adds the path from the APPEND_TO_PATH env vars to the PATH for the command", func() {
				pathString := os.Getenv("PATH") + ":/other/packages:/and/more/dirs"
				h := OsHelperImpl{}
				cmd, err := h.StartCommand("/tmp/some-logpath", "echo")
				Expect(err).NotTo(HaveOccurred())
				Expect(cmd.Env).To(ContainElement("PATH=" + pathString))
			})

			It("Does not duplicate the PATH in the Env object", func() {
				h := OsHelperImpl{}
				cmd, err := h.StartCommand("/tmp/some-logpath", "echo")
				Expect(err).NotTo(HaveOccurred())
				pathCount := 0
				for _, env := range cmd.Env {
					if strings.HasPrefix(env, "PATH=") {
						pathCount += 1
					}
				}
				Expect(pathCount).To(Equal(1))
			})

			AfterEach(func() {
				os.Unsetenv("APPEND_TO_PATH")
			})
		})

		Context("when APPEND_TO_PATH is not set", func() {
			It("does not change the path", func() {
				h := OsHelperImpl{}
				cmd, err := h.StartCommand("/tmp/some-logpath", "echo")
				Expect(err).NotTo(HaveOccurred())
				Expect(cmd.Env).To(ContainElement(Equal("PATH=" + os.Getenv("PATH"))))
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
})
