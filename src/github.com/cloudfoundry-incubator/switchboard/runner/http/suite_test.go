package http

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestHTTPRunner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "HTTP Runner Test Suite")
}
