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
	_ = BeforeAll(func() {
		net, aliases := testhelper.CreateTestNetwork()
		pass := uuid.New().String()
		_, sourceFromHost = testhelper.StartContainerInstance("dumpTest", pass, aliases, net)
		_, targetFromHost = testhelper.StartContainerInstance("restoreTest", pass, aliases, net)
		var err error
		dumpClient, err = dumper.New(sourceFromHost, dataDir, mysqlBinDir)
		Expect(err).ToNot(HaveOccurred())
	})
	It("creates a	backup", func() {
		testhelper.GenerateTestData(sourceFromHost, "dumpDB", "dumpTbl", 10)
		var err error
		dumpPath, err = dumpClient.Dump("test.sql")
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
