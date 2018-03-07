package db_helper_test

import (
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

func TestDB_helper(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "DB Helper Suite")
}
