package utils_test

import (
	"github.com/cloudfoundry/pxc-release/replicator/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Utils", func() {
	expectedVal := "6ab3825e-e60b-11ed-a160-0242ac120002:1-15432"
	DescribeTable("parsing gtids",
		func(line string, shouldMatch bool) {
			val, found := utils.ParseGTIDFromLine(line)
			Expect(found).To(Equal(shouldMatch))
			if shouldMatch {
				Expect(val).To(Equal(expectedVal))
			}
		},
		Entry("with comment", "SET @@GLOBAL.GTID_PURGED=/*!80000 '+'*/ '6ab3825e-e60b-11ed-a160-0242ac120002:1-15432';", true),
		Entry("without comment", "SET @@GLOBAL.GTID_PURGED='6ab3825e-e60b-11ed-a160-0242ac120002:1-15432';", true),
		Entry("without the SET", "GLOBAL.GTID_PURGED='6ab3825e-e60b-11ed-a160-0242ac120002:1-15432';", false),
		Entry("missing 2nd @", "SET @GLOBAL.GTID_PURGED='6ab3825e-e60b-11ed-a160-0242ac120002:1-15432';", false),
		Entry("missing 2nd gtid element", "@@GLOBAL.GTID_PURGED='6ab3825e-e60b-11ed-a160-0242ac120002';", false),
		Entry("not a valid uuid", "SET @GLOBAL.GTID_PURGED='6ab3825e-11ed-a160-0242ac120002:1-15432';", false),
		Entry("double range 2nd element", "SET @GLOBAL.GTID_PURGED='6ab3825e-e60b-11ed-a160-0242ac120002:1-15432:1-22';", false),
	)
})
