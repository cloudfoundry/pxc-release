package config_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestGaleraHealthcheckConfig(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Galera Healthcheck Config Suite")
}
