package cmdopts_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestCmdopts(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Cmdopts Suite")
}
