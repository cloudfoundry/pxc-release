package node_prestarter_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestNodePreStarter(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NodePreStarter Suite")
}
