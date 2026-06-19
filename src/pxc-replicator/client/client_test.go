package client_test

import (
	"encoding/json"
	"log"
	"time"

	"github.com/cloudfoundry/pxc-release/replicator/client"
	"github.com/cloudfoundry/pxc-release/replicator/config"
	"github.com/cloudfoundry/pxc-release/replicator/testhelper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Client/Client", func() {
	//FIt("parses empty time", func() {
	//	t, err := time.Parse(client.DATE_LAYOUT, "")
	//	Expect(err).To(BeNil())
	//	Expect(t).To(BeNil())
	//})
	var replClient client.ReplClient
	var source, sourceFromHost, _, targetFromHost config.Target
	Describe("updating creds after sync", Ordered, func() {
		_ = BeforeAll(func() {
			testNet, aliases := testhelper.CreateTestNetwork()

			source, sourceFromHost = testhelper.StartContainerInstance("source", testhelper.GeneratePassword(), aliases, testNet)
			_, targetFromHost = testhelper.StartContainerInstance("target", testhelper.GeneratePassword(), aliases, testNet)

			replClient = client.ReplClient{
				Source:  sourceFromHost,
				Target:  targetFromHost,
				DataDir: dataDir,
				BinDir:  mysqlBinDir,
			}
		})

		It("will reconnect with the synced creds", func() {
			Expect(replClient.SyncSourceToTarget()).To(Succeed())

			db, err := replClient.ConnectTarget()
			Expect(err).ToNot(HaveOccurred())
			replClient.Source = source
			Expect(replClient.Configure(db)).To(Succeed())
			state, err := replClient.CheckReplication(db)
			Expect(err).ToNot(HaveOccurred())
			stateJSONBytes := []byte{}
			client.CloseAndLogError(db)
			db, err = replClient.ConnectTarget()
			Expect(err).ToNot(HaveOccurred())
			Eventually(func() bool {
				state, err = replClient.CheckReplication(db)
				Expect(err).ToNot(HaveOccurred())
				stateJSONBytes, err = json.MarshalIndent(state, "", "  ")
				log.Default().Printf("%v", string(stateJSONBytes))
				time.Sleep(time.Second)
				return state.SQLRunning == "Yes" && state.IORunning == "Yes"
			}, time.Minute).Should(BeTrue())
			db, err = replClient.ConnectTarget()
			defer Expect(db.Close()).To(Succeed())
			Expect(err).ToNot(HaveOccurred())
		})
	})
	FDescribe("full start procedure", Ordered, func() {
		_ = BeforeAll(func() {
			testNet, aliases := testhelper.CreateTestNetwork()

			source, sourceFromHost = testhelper.StartContainerInstance(testhelper.GeneratePassword(), "test", aliases, testNet)
			_, targetFromHost = testhelper.StartContainerInstance(testhelper.GeneratePassword(), "test", aliases, testNet)

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
			defer client.CloseAndLogError(db)
			Expect(err).ToNot(HaveOccurred())
			db, err = replClient.ConnectTarget()
			defer client.CloseAndLogError(db)
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
