package thermostat_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestHotsqlRelease(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Thermostat")
}
