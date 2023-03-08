package bridge_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestBridge(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Bridge Runner Suite")
}
