package monit_status_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestGalera_monitStatus(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "MonitStatus Suite")
}
