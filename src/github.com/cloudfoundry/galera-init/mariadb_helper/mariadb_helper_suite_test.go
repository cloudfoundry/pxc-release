package mariadb_helper_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestMariadb_helper(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "MariaDB Helper Suite")
}
