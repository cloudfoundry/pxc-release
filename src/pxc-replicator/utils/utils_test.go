package utils_test

import (
	"github.com/cloudfoundry/pxc-release/replicator/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Utils", func() {
	DescribeTable("parsing gtids",
		func(line string, shouldMatch bool, expected string) {
			val, found := utils.ParseGTIDFromLine(line)
			Expect(found).To(Equal(shouldMatch))
			if shouldMatch {
				Expect(val).To(Equal(expected))
			}
		},
		Entry("with multiple ranges single uuid", "SET @@GLOBAL.GTID_PURGED=/*!80000 '+'*/ '6ab3825e-e60b-11ed-a160-0242ac120002:1-15432:2-85852';", true, "6ab3825e-e60b-11ed-a160-0242ac120002:1-15432:2-85852"),
		Entry("with multiple ranges separate uuid", "SET @@GLOBAL.GTID_PURGED=/*!80000 '+'*/ '6ab3825e-e60b-11ed-a160-0242ac120002:1-15432,6ab3825e-e60b-11ed-a160-0242ac120003:2-85852';", true, "6ab3825e-e60b-11ed-a160-0242ac120002:1-15432,6ab3825e-e60b-11ed-a160-0242ac120003:2-85852"),
		Entry("with multiple ranges separate uuid and space", "SET @@GLOBAL.GTID_PURGED=/*!80000 '+'*/ '6ab3825e-e60b-11ed-a160-0242ac120002:1-15432, 6ab3825e-e60b-11ed-a160-0242ac120003:2-85852';", true, "6ab3825e-e60b-11ed-a160-0242ac120002:1-15432,6ab3825e-e60b-11ed-a160-0242ac120003:2-85852"),
		Entry("with comment", "SET @@GLOBAL.GTID_PURGED=/*!80000 '+'*/ '6ab3825e-e60b-11ed-a160-0242ac120002:1-15432';", true, "6ab3825e-e60b-11ed-a160-0242ac120002:1-15432"),
		Entry("without comment", "SET @@GLOBAL.GTID_PURGED='6ab3825e-e60b-11ed-a160-0242ac120002:1-15432';", true, "6ab3825e-e60b-11ed-a160-0242ac120002:1-15432"),
		Entry("without the SET", "GLOBAL.GTID_PURGED='6ab3825e-e60b-11ed-a160-0242ac120002:1-15432';", false, "6ab3825e-e60b-11ed-a160-0242ac120002:1-15432"),
		Entry("missing 2nd @", "SET @GLOBAL.GTID_PURGED='6ab3825e-e60b-11ed-a160-0242ac120002:1-15432';", false, "6ab3825e-e60b-11ed-a160-0242ac120002:1-15432"),
		Entry("missing 2nd gtid element", "@@GLOBAL.GTID_PURGED='6ab3825e-e60b-11ed-a160-0242ac120002';", false, "6ab3825e-e60b-11ed-a160-0242ac120002:1-15432"),
		Entry("not a valid uuid", "SET @GLOBAL.GTID_PURGED='6ab3825e-11ed-a160-0242ac120002:1-15432';", false, "6ab3825e-e60b-11ed-a160-0242ac120002:1-15432"),
		Entry("double range 2nd element", "SET @GLOBAL.GTID_PURGED='6ab3825e-e60b-11ed-a160-0242ac120002:1-15432:1-22';", false, "6ab3825e-e60b-11ed-a160-0242ac120002:1-15432"),
	)
})
