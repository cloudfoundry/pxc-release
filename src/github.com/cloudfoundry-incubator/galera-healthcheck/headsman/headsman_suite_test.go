package headsman_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestHeadsman(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Headsman Suite")
}
