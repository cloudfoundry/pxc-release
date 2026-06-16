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
	var fromHost config.Target
	_ = BeforeAll(func() {
		net, aliases := testhelper.CreateTestNetwork()
		pass := uuid.New().String()
		_, fromHost = testhelper.StartContainerInstance("dumpTest", pass, aliases, net)
	})
	It("creates a	backup", func() {
		dumpClient, err := dumper.New(fromHost, "/tmp/replicator", "/nix/store/vikhrsnr4cbrmgky1lk8k3xdayag5278-mysql-8.4.9/bin")
		Expect(err).ToNot(HaveOccurred())
		dumpPath, err := dumpClient.Dump("test.sql")
		Expect(err).ToNot(HaveOccurred())
		Expect(dumpPath).ToNot(BeEmpty())
		bytes, err := os.ReadFile(dumpPath)
		Expect(err).ToNot(HaveOccurred())
		Expect(bytes).To(ContainSubstring("Dump completed on"))
	})
})
