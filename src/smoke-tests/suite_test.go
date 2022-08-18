package smoke_test

import (
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestConnection(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "PXC Smoke Tests")
}

var _ = BeforeSuite(func() {
	Expect(os.Getenv("PROXY_HOST")).NotTo(BeEmpty(),
		`Missing environment variable: MYSQL_HOST`)
	Expect(os.Getenv("MYSQL_HOSTS")).NotTo(BeEmpty(),
		`Missing environment variable: MYSQL_HOST`)
	Expect(os.Getenv("MYSQL_USERNAME")).NotTo(BeEmpty(),
		`Missing environment variable: MYSQL_USERNAME`)
	Expect(os.Getenv("MYSQL_PASSWORD")).NotTo(BeEmpty(),
		`Missing environment variable: MYSQL_PASSWORD`)
})
