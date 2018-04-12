package unpack_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestGpg(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Gpg Suite")
}
