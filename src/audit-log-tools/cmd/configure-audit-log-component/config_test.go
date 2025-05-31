package main_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"auditlogtools/cmd/configure-audit-log-component"
)

var _ = Describe("ParseConfig", func() {
	BeforeEach(func() {
		GinkgoT().Setenv("MYSQL_DSN", "user:password@tcp(127.0.0.1:3306)/mysql")
		GinkgoT().Setenv("MYSQL_AUDIT_EXCLUDE_USERS", "user1@localhost,user2@%,user3@specific.remote.ip")
	})

	It("parses config", func() {
		var cfg main.Config

		err := main.ParseConfig(&cfg)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.MySQL.DB).NotTo(BeNil())
		Expect(cfg.MySQL.Cfg.User).To(Equal("user"))
		Expect(cfg.MySQL.Cfg.Passwd).To(Equal("password"))
		Expect(cfg.MySQL.Cfg.Net).To(Equal("tcp"))
		Expect(cfg.MySQL.Cfg.Addr).To(Equal("127.0.0.1:3306"))
		Expect(cfg.MySQL.Cfg.DBName).To(Equal("mysql"))
		Expect(cfg.MySQL.Cfg.InterpolateParams).To(BeTrue(), `client side prepared statements should be enabled, but they are not`)
		Expect(cfg.MySQL.Cfg.AllowNativePasswords).To(BeTrue(), `mysql_native_password support should be enabled`)

		Expect(cfg.ExcludeUsers).To(ConsistOf(
			`user1@localhost`,
			`user2@%`,
			`user3@specific.remote.ip`,
		))
	})

	When("an invalid MySQL DSN is provided", func() {
		BeforeEach(func() {
			GinkgoT().Setenv("MYSQL_DSN", "some-invalid-DSN")
		})
		It("returns an error", func() {
			var cfg main.Config

			err := main.ParseConfig(&cfg)
			Expect(err).To(MatchError(ContainSubstring("invalid MySQL DSN")))
		})
	})
})
