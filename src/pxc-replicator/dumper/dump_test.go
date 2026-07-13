package dumper_test

import (
	"fmt"
	"os"

	"github.com/cloudfoundry/pxc-release/replicator/client"
	"github.com/cloudfoundry/pxc-release/replicator/config"
	"github.com/cloudfoundry/pxc-release/replicator/dumper"
	"github.com/cloudfoundry/pxc-release/replicator/testhelper"
	"github.com/cloudfoundry/pxc-release/replicator/utils"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Dumper/Dump", Ordered, func() {
	var sourceFromHost, targetFromHost config.Target
	var dumpClient dumper.Dumper
	var dumpPath string
	dataDir, err := os.MkdirTemp("", "")
	Expect(err).ToNot(HaveOccurred())
	dumpDir, err := os.MkdirTemp("", "")
	Expect(err).ToNot(HaveOccurred())

	expectedTable := "someTableName"
	Describe("with encryption", FlakeAttempts(3), func() {
		_ = BeforeAll(func() {
			net := testhelper.CreateTestNetwork()
			aliases := []string{"localhost"}
			pass := uuid.New().String()
			tag := "8.0"
			_, sourceFromHost, _ = testhelper.StartPXCInstance(pass, tag, testhelper.VerifyCA, aliases, net)
			testhelper.GenerateTestData(sourceFromHost, "dumpDB", expectedTable, 10)
			sourceFromHost.Creds.AdminUsername, sourceFromHost.Creds.AdminPassword = "", ""
			_, targetFromHost, _ = testhelper.StartPXCInstance(pass, tag, testhelper.VerifyCA, aliases, net)
			var err error

			r := client.ReplClient{
				Source:  sourceFromHost,
				Target:  targetFromHost,
				DataDir: dataDir,
				DumpDir: dumpDir,
			}
			Expect(r.InitFiles()).To(Succeed())
			dumpClient, err = dumper.New(sourceFromHost, dumpDir, dataDir, testhelper.MysqlBinDir)
			Expect(err).ToNot(HaveOccurred())
		})
		It("creates a	backup", func() {
			var err error
			dumpPath, err = dumpClient.Dump()
			Expect(err).ToNot(HaveOccurred())
			Expect(dumpPath).ToNot(BeEmpty())
			bytes, err := os.ReadFile(dumpPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(bytes).To(ContainSubstring("Dump completed on"))
			Expect(bytes).To(MatchRegexp(fmt.Sprintf("CREATE TABLE `%s`", expectedTable)))
		})
		It("restores a	backup", func() {
			err := dumpClient.Restore(dumpPath, targetFromHost)
			Expect(err).ToNot(HaveOccurred())
		})
	})
	Describe("without encryption", FlakeAttempts(3), func() {
		tableName := "testTable"
		_ = BeforeAll(func() {
			net := testhelper.CreateTestNetwork()

			pass := uuid.New().String()
			tag := "8.0"
			_, sourceFromHost, _ = testhelper.StartPXCInstance(pass, tag, testhelper.TLSDisabled, []string{"localhost"}, net)
			sourceFromHost.Creds.AdminUsername, sourceFromHost.Creds.AdminPassword = "", ""
			_, targetFromHost, _ = testhelper.StartPXCInstance(pass, tag, testhelper.TLSDisabled, []string{"localhost"}, net)
			var err error

			_, err = utils.WriteMysqlCnf(sourceFromHost, dataDir, false)
			Expect(err).ToNot(HaveOccurred())

			_, err = utils.WriteMysqlCnf(targetFromHost, dataDir, false)
			Expect(err).ToNot(HaveOccurred())

			Expect(utils.WriteCertFiles(sourceFromHost, dataDir)).To(Succeed())
			Expect(utils.WriteCertFiles(targetFromHost, dataDir)).To(Succeed())

			dumpClient, err = dumper.New(sourceFromHost, dumpDir, dataDir, testhelper.MysqlBinDir)

			testhelper.GenerateTestData(sourceFromHost, "testDataBase", tableName, 1000)
			Expect(err).ToNot(HaveOccurred())
		})
		It("creates a	backup", func() {
			testhelper.GenerateTestData(sourceFromHost, "dumpDB", expectedTable, 10)
			var err error
			dumpPath, err = dumpClient.Dump()
			Expect(err).ToNot(HaveOccurred())
			Expect(dumpPath).ToNot(BeEmpty())
			bytes, err := os.ReadFile(dumpPath)
			Expect(err).ToNot(HaveOccurred())
			Expect(bytes).To(ContainSubstring("Dump completed on"))
			Expect(bytes).To(MatchRegexp(fmt.Sprintf("CREATE TABLE.*%s", tableName)))
		})
		It("restores a	backup", func() {
			err := dumpClient.Restore(dumpPath, targetFromHost)
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
