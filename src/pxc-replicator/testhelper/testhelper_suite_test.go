package testhelper_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTesthelper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Testhelper Suite")
}
