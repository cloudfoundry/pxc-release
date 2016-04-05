package preparer_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestPreparer(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Preparer Suite")
}
