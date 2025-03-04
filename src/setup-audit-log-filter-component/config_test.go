package main

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ParseConfig", func() {
	BeforeEach(func() {
		GinkgoT().Setenv("MYSQL_DSN", "user:password@tcp(127.0.0.1:3306)/mysql")
	})

	It("parses config", func() {
		var cfg Config

		err := ParseConfig(&cfg)
		Expect(err).NotTo(HaveOccurred())

		Expect(cfg.MySQL.DB).NotTo(BeNil())
		Expect(cfg.MySQL.cfg.User).To(Equal("user"))
		Expect(cfg.MySQL.cfg.Passwd).To(Equal("password"))
		Expect(cfg.MySQL.cfg.Net).To(Equal("tcp"))
		Expect(cfg.MySQL.cfg.Addr).To(Equal("127.0.0.1:3306"))
		Expect(cfg.MySQL.cfg.DBName).To(Equal("mysql"))
		Expect(cfg.MySQL.cfg.InterpolateParams).To(BeTrue(), `client side prepared statements should be enabled, but they are not`)
		Expect(cfg.MySQL.cfg.AllowNativePasswords).To(BeTrue(), `mysql_native_password support should be enabled`)
	})
})
