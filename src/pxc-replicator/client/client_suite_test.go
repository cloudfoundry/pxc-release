package client_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	mysqlBinDir = os.Getenv("MYSQL_BIN_DIR")
	dataDir     = os.Getenv("DATA_DIR")
)

func TestClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Client Suite")
}
