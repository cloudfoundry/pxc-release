package start_manager_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestStartManager(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Start Manager Suite")
}
