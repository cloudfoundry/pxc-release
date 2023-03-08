package node_manager_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNodeManager(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Node Manager Suite")
}
