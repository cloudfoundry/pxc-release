package sequence_number_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGalera_sequence_number(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Sequence number Suite")
}
