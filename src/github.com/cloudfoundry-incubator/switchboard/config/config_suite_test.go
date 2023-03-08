package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestSwitchboarConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Switchboard Config Suite")
}
