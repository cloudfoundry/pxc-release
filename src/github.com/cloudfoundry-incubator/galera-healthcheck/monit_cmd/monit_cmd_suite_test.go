package monit_cmd_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestMonit_Cmd(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Monit Cmd Suite")
}
