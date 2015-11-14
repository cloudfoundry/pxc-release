package start_mysql_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGalera_StartMysql(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "StartMySql Suite")
}
