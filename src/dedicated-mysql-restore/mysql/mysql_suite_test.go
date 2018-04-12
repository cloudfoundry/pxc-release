package mysql_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestMysql(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Mysql Suite")
}
