package node_runner_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestNodeRunner(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Node Runner Suite")
}
