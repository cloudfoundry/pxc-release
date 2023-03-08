package galera_init_status_server_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestServiceStatusServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GaleraInitStatusServer Suite")
}
