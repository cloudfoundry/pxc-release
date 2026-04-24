package bpm_client_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestBmpClient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "BmpClient Suite")
}