package client_test

import (
	"log"
	"os"

	"github.com/cloudfoundry/pxc-release/replicator/client"
	"github.com/cloudfoundry/pxc-release/replicator/config"
	"github.com/cloudfoundry/pxc-release/replicator/dumper"
	"github.com/cloudfoundry/pxc-release/replicator/testhelper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Client/Client", func() {
	var replClient client.ReplClient
	var source, sourceFromHost, _, targetFromHost config.Target
	Describe("full start procedure", Ordered, func() {
		_ = BeforeAll(func() {
			testNet, aliases := testhelper.CreateTestNetwork()

			source, sourceFromHost = testhelper.StartContainerInstance("source", "test", aliases, testNet)
			_, targetFromHost = testhelper.StartContainerInstance("target", "test", aliases, testNet)

			dumpClient, err := dumper.New(sourceFromHost, dataDir, mysqlBinDir)
			Expect(err).ToNot(HaveOccurred())
			tempDump, err := os.CreateTemp("/tmp", "testdump")
			Expect(err).ToNot(HaveOccurred())
			dumpClient.Dump(tempDump.Name())
			Expect(dumpClient.Restore(tempDump.Name(), targetFromHost)).To(Succeed())

			replClient = client.ReplClient{
				Source: sourceFromHost,
				Target: targetFromHost,
			}
		})
		It("can connect with the provided creds", func() {
			Expect(sourceFromHost.Host).ToNot(BeEmpty())
			db, err := replClient.ConnectSource()
			defer client.CloseAndLogError(db)
			Expect(err).ToNot(HaveOccurred())
			db, err = replClient.ConnectTarget()
			defer client.CloseAndLogError(db)
			Expect(err).ToNot(HaveOccurred())
		})

		It("can configure replication on the target", func() {
			// the source should use the "container IP" for this test,
			// else the replica will try to connect to localhost:<dynPort> and fail...
			replClient.Source = source
			db, err := replClient.ConnectTarget()
			defer client.CloseAndLogError(db)
			Expect(err).ToNot(HaveOccurred())
			Expect(replClient.Configure(db)).To(Succeed())
			Expect(db.Close()).To(Succeed())
		})
		It("gets the replication state", func() {
			db, err := replClient.ConnectTarget()
			defer client.CloseAndLogError(db)
			Expect(err).ToNot(HaveOccurred())
			testhelper.GenerateTestData(targetFromHost, "first", "moredata", 100)
			state, err := replClient.CheckReplication(db)

			Expect(err).ToNot(HaveOccurred())
			Expect(state).ToNot(Equal(client.ReplState{}))
			Expect(db.Close()).To(Succeed())
			Expect(state.SQLRunning).To(Equal("Yes"))
			Expect(state.IORunning).To(Equal("Yes"))
			Expect(state.Misc).ToNot(BeEmpty())
			log.Default().Printf("%v", state.Misc)
		})
	})
})
