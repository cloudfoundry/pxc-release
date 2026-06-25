package dumper_test

import (
	"os"

	"github.com/cloudfoundry/pxc-release/replicator/config"
	"github.com/cloudfoundry/pxc-release/replicator/dumper"
	"github.com/cloudfoundry/pxc-release/replicator/testhelper"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Dumper/Dump", Ordered, func() {
	var sourceFromHost, targetFromHost config.Target
	var dumpClient dumper.Dumper
	var dumpPath string
	Describe("with encryption", func() {
		_ = BeforeAll(func() {
			net := testhelper.CreateTestNetwork()
			aliases := []string{"localhost"}
			pass := uuid.New().String()
			tag := "8.0"
			_, sourceFromHost, _ = testhelper.StartContainerInstance("dumpTest", pass, tag, testhelper.VerifyCA, aliases, net)
			_, targetFromHost, _ = testhelper.StartContainerInstance("restoreTest", pass, tag, testhelper.VerifyCA, aliases, net)
			var err error
			dumpClient, err = dumper.New(sourceFromHost, testhelper.DataDir, testhelper.MysqlBinDir)
			Expect(err).ToNot(HaveOccurred())
		})
		It("creates a	backup", func() {
			testhelper.GenerateTestData(sourceFromHost, "dumpDB", "dumpTbl", 10)
			var err error
			dumpPath, err = dumpClient.Dump()
			Expect(err).ToNot(HaveOccurred())
			Expect(dumpPath).ToNot(BeEmpty())
			bytes, err := os.ReadFile(dumpPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(bytes).To(ContainSubstring("Dump completed on"))
		})
		It("restores a	backup", func() {
			err := dumpClient.Restore(dumpPath, targetFromHost)
			Expect(err).ToNot(HaveOccurred())
		})
	})
	Describe("without encryption", func() {
		_ = BeforeAll(func() {
			net := testhelper.CreateTestNetwork()

			pass := uuid.New().String()
			tag := "8.0"
			_, sourceFromHost, _ = testhelper.StartContainerInstance("dumpTest", pass, tag, testhelper.TLSDisabled, []string{"localhost"}, net)
			_, targetFromHost, _ = testhelper.StartContainerInstance("restoreTest", pass, tag, testhelper.TLSDisabled, []string{"localhost"}, net)
			var err error
			dumpClient, err = dumper.New(sourceFromHost, testhelper.DataDir, testhelper.MysqlBinDir)
			Expect(err).ToNot(HaveOccurred())
		})
		It("creates a	backup", func() {
			testhelper.GenerateTestData(sourceFromHost, "dumpDB", "dumpTbl", 10)
			var err error
			dumpPath, err = dumpClient.Dump()
			Expect(err).ToNot(HaveOccurred())
			Expect(dumpPath).ToNot(BeEmpty())
			bytes, err := os.ReadFile(dumpPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(bytes).To(ContainSubstring("Dump completed on"))
		})
		It("restores a	backup", func() {
			err := dumpClient.Restore(dumpPath, targetFromHost)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
