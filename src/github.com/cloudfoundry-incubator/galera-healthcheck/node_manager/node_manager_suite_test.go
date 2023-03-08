package node_manager_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestServiceManager(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Service Manager Suite")
}
