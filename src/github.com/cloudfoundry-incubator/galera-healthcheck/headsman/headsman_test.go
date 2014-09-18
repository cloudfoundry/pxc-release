package headsman_test

import (
	"github.com/cloudfoundry-incubator/galera-healthcheck/headsman"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper/fakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Headsman", func() {
	It("Calls a script with the given parameters", func() {
		fakeOsHelper := fakes.FakeOsHelper{}
		h := headsman.NewMysqlHeadsman(&fakeOsHelper, "username", "password", "path", "ip")

		h.Chop()

		executablePath, args := fakeOsHelper.RunCommandArgsForCall(0)
		Expect(executablePath).To(Equal("path"))
		Expect(args).To(Equal([]string{"username", "password", "ip"}))
	})
})
