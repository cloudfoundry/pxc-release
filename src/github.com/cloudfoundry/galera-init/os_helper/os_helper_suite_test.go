package os_helper_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestOs_helper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OS Helper Suite")
}
