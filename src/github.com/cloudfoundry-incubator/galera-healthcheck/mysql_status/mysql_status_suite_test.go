package mysql_status_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGalera_MySQLStatus(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "MySQL status Suite")
}
