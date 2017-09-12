package os_helper_test

import (
	"os/exec"

	. "github.com/cloudfoundry/mariadb_ctrl/os_helper"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("OsHelper", func() {
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
