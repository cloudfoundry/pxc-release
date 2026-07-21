package config_test

import (
	"github.com/cloudfoundry/pxc-release/replicator/config"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Target", func() {
	base := config.Target{
		Name:    "test",
		Host:    "1.2.3.4",
		Port:    3306,
		Creds:   config.Creds{Username: "root", Password: "secret"},
		Certs:   config.Certs{},
		Version: "8.0",
	}

	Describe("DSN", func() {
		DescribeTable("formats the DSN correctly",
			func(t config.Target, expected string) {
				Expect(t.DSN()).To(Equal(expected))
			},
			Entry("basic", base, "root:secret@tcp(1.2.3.4:3306)/"),
			Entry("empty username", config.Target{Host: "h", Port: 3306, Creds: config.Creds{Password: "p"}}, ":p@tcp(h:3306)/"),
			Entry("empty password", config.Target{Host: "h", Port: 3306, Creds: config.Creds{Username: "u"}}, "u:@tcp(h:3306)/"),
			Entry("port 0", config.Target{Host: "h", Port: 0, Creds: config.Creds{Username: "u", Password: "p"}}, "u:p@tcp(h:0)/"),
		)
	})

	Describe("AdminDSN", func() {
		DescribeTable("formats the admin DSN correctly",
			func(t config.Target, expected string) {
				Expect(t.AdminDSN()).To(Equal(expected))
			},
			Entry("with admin creds", config.Target{
				Host: "h", Port: 3306,
				Creds: config.Creds{AdminUsername: "admin", AdminPassword: "pass"},
			}, "admin:pass@tcp(h:3306)/"),
			Entry("empty admin creds", config.Target{
				Host: "h", Port: 3306,
				Creds: config.Creds{Username: "u", Password: "p"},
			}, ":@tcp(h:3306)/"),
		)
	})

	Describe("String", func() {
		DescribeTable("redacts the password",
			func(t config.Target, expected string) {
				Expect(t.String()).To(Equal(expected))
			},
			Entry("basic", base, "root:<redacted>@tcp(1.2.3.4:3306)"),
			Entry("empty username", config.Target{Host: "h", Port: 3306, Creds: config.Creds{Password: "p"}}, ":<redacted>@tcp(h:3306)"),
		)
	})
})
