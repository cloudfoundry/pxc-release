package statuslogger_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestStatusLogger(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Status Logger Suite")
}

