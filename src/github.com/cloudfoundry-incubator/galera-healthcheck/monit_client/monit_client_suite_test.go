package monit_client_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGalera_StartMonit(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Monit Client Suite")
}
