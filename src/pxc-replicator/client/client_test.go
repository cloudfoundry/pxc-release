package client_test

import (
	"encoding/json"
	"log"
	"time"

	"github.com/cloudfoundry/pxc-release/replicator/client"
	"github.com/cloudfoundry/pxc-release/replicator/config"
	"github.com/cloudfoundry/pxc-release/replicator/testhelper"
	"github.com/cloudfoundry/pxc-release/replicator/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Client/Client", func() {
	var replClient client.ReplClient
	var source, sourceFromHost, _, targetFromHost config.Target

	Describe("using tls connections", Ordered, func() {
		_ = BeforeAll(func() {
			testNet := testhelper.CreateTestNetwork()

			source, sourceFromHost, _ = testhelper.StartContainerInstance(testhelper.GeneratePassword(), testhelper.GeneratePassword(), "8.4", testhelper.VerifyCA, []string{"source"}, testNet)
			replClient = client.ReplClient{
				Source:  sourceFromHost,
				Target:  config.Target{},
				DataDir: dataDir,
				BinDir:  mysqlBinDir,
			}
		})
		It("will work", func() {
			db, err := replClient.ConnectSource()
			Expect(err).To(BeNil())
			Expect(db.Close()).To(Succeed())
		})
	})

	Describe("mismatched versions", Ordered, func() {
		_ = BeforeAll(func() {
			testNet := testhelper.CreateTestNetwork()

			source, sourceFromHost, _ = testhelper.StartContainerInstance(testhelper.GeneratePassword(), testhelper.GeneratePassword(), "8.0", testhelper.TLSDisabled, []string{"source"}, testNet)
			_, targetFromHost, _ = testhelper.StartContainerInstance(testhelper.GeneratePassword(), testhelper.GeneratePassword(), "8.4", testhelper.TLSDisabled, []string{"target"}, testNet)
			replClient = client.ReplClient{
				Source:  sourceFromHost,
				Target:  targetFromHost,
				DataDir: dataDir,
				BinDir:  mysqlBinDir,
			}
		})

		It("will generate an error about mismatched versions", func() {
			Expect(replClient.CheckVersion()).To(MatchError(MatchRegexp("sourceVersion: 8.0 does not match targetVersion: 8.4")))
		})
	})
	Describe("checking if replication is enablded", func() {
		_ = BeforeEach(func() {
			testNet := testhelper.CreateTestNetwork()

			_, targetFromHost, _ = testhelper.StartContainerInstance(testhelper.GeneratePassword(), testhelper.GeneratePassword(), "8.4", testhelper.TLSDisabled, []string{"target"}, testNet)
			replClient = client.ReplClient{
				Target:  targetFromHost,
				DataDir: dataDir,
				BinDir:  mysqlBinDir,
			}
		})

		It("will return state with Enabled=false", func() {
			db, err := replClient.ConnectTarget()
			Expect(err).ToNot(HaveOccurred())
			defer utils.CloseAndLogError(db)
			state, err := replClient.CheckReplication(db)
			Expect(err).ToNot(HaveOccurred())
			Expect(state.Enabled).To(BeFalse())
		})
	})
	Describe("updating creds through syncing initial state", Ordered, func() {
		_ = BeforeAll(func() {
			testNet := testhelper.CreateTestNetwork()

			source, sourceFromHost, _ = testhelper.StartContainerInstance(testhelper.GeneratePassword(), testhelper.GeneratePassword(), testhelper.Tag, testhelper.TLSDisabled, []string{"source"}, testNet)
			_, targetFromHost, _ = testhelper.StartContainerInstance(testhelper.GeneratePassword(), testhelper.GeneratePassword(), testhelper.Tag, testhelper.TLSDisabled, []string{"target"}, testNet)

			replClient = client.ReplClient{
				Source:  sourceFromHost,
				Target:  targetFromHost,
				DataDir: dataDir,
				BinDir:  mysqlBinDir,
			}
		})

		It("will work because the connection is already established", func() {
			endToEnd(replClient, source)
		})
	})
	Describe("full start procedure", Ordered, func() {
		_ = BeforeAll(func() {
			testNet := testhelper.CreateTestNetwork()

			source, sourceFromHost, _ = testhelper.StartContainerInstance(testhelper.GeneratePassword(), "test", testhelper.Tag, testhelper.TLSDisabled, []string{"source"}, testNet)
			_, targetFromHost, _ = testhelper.StartContainerInstance(testhelper.GeneratePassword(), "test", testhelper.Tag, testhelper.TLSDisabled, []string{"target"}, testNet)

			replClient = client.ReplClient{
				Source:  sourceFromHost,
				Target:  targetFromHost,
				DataDir: dataDir,
				BinDir:  mysqlBinDir,
			}
		})
		It("can connect with the provided creds", func() {
			Expect(sourceFromHost.Host).ToNot(BeEmpty())
			db, err := replClient.ConnectSource()
			defer utils.CloseAndLogError(db)
			Expect(err).ToNot(HaveOccurred())
			db, err = replClient.ConnectTarget()
			defer utils.CloseAndLogError(db)
			Expect(err).ToNot(HaveOccurred())
		})
		It("will dump and import the source", func() {
			replClient.Source = sourceFromHost
			Expect(replClient.SyncSourceToTarget()).To(Succeed())
		})
		It("can configure replication on the target", func() {
			// the source should use the "container IP" for this test,
			// else the replica will try to connect to localhost:<dynPort> and fail...
			replClient.Source = source
			db, err := replClient.ConnectTarget()
			defer utils.CloseAndLogError(db)
			Expect(err).ToNot(HaveOccurred())
			Expect(replClient.Configure(db)).To(Succeed())
			Expect(db.Close()).To(Succeed())
		})
		It("gets the replication state", func() {
			db, err := replClient.ConnectTarget()
			defer utils.CloseAndLogError(db)
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
		It("will have the same values in the replica", func() {
			db, err := replClient.ConnectTarget()
			Expect(err).ToNot(HaveOccurred())
			rows, err := db.Query("select * from first.moredata")
			Expect(err).ToNot(HaveOccurred())
			data := testhelper.TestDataRow{}
			total := 0
			for rows.Next() {
				total += 1
				Expect(rows.Scan(&data.ID, &data.CreatedAt, &data.UpdatedAt, &data.Value)).To(Succeed())
				Expect(data.Value).ToNot(BeNil())
				Expect(data.CreatedAt).ToNot(BeNil())
				Expect(data.UpdatedAt).To(BeNil())
			}
			Expect(total).To(Equal(100))
		})
	})
})

func endToEnd(replClient client.ReplClient, source config.Target) {
	Expect(replClient.SyncSourceToTarget()).To(Succeed())
	db, err := replClient.ConnectTarget()
	Expect(err).ToNot(HaveOccurred())
	replClient.Source = source
	Expect(replClient.Configure(db)).To(Succeed())
	state, err := replClient.CheckReplication(db)
	Expect(err).ToNot(HaveOccurred())
	stateJSONBytes := []byte{}
	Eventually(func() bool {
		state, err = replClient.CheckReplication(db)
		Expect(err).ToNot(HaveOccurred())
		stateJSONBytes, err = json.MarshalIndent(state, "", "  ")
		log.Default().Printf("%v", string(stateJSONBytes))
		time.Sleep(time.Second)
		return state.SQLRunning == "Yes" && state.IORunning == "Yes"
	}, time.Minute).Should(BeTrue())
	Expect(err).ToNot(HaveOccurred())
	Expect(db.Close()).To(Succeed())
}
