package main_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestGaleraInit(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Start Executable Suite")
}
