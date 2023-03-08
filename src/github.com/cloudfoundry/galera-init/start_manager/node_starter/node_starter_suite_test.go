package node_starter_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestNodeStarter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NodeStarter Suite")
}
