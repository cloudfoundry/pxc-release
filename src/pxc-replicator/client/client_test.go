package client_test

import (
	"context"
	"fmt"

	"github.com/cloudfoundry/pxc-release/replicator/client"
	"github.com/cloudfoundry/pxc-release/replicator/config"
	"github.com/cloudfoundry/pxc-release/replicator/testhelper"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/network"
)

var _ = Describe("Client/Client", func() {
	var replClient, replClientFromHost client.ReplClient
	var source, sourceFromHost, target, targetFromHost config.Target

	Describe("establishing connections", Ordered, func() {
		_ = BeforeAll(func() {
			ctx := context.Background()
			newNetwork, err := network.New(ctx)
			Expect(err).ToNot(HaveOccurred())
			testcontainers.CleanupNetwork(ginkgo.GinkgoTB(), newNetwork)
			aliases := []string{"the_docs_told_me_to_create_one"}
			source, sourceFromHost = testhelper.StartContainerInstance("source", "test", aliases, newNetwork)
			fmt.Printf("source: %v\n", source)
			target, targetFromHost = testhelper.StartContainerInstance("target", "test", aliases, newNetwork)
			fmt.Printf("target: %v\n", targetFromHost)
			testhelper.GenerateTestData(sourceFromHost, "first", "data", 100)
		})
		It("can connect with the provided creds", func() {
			Expect(sourceFromHost.Host).ToNot(BeEmpty())
			replClient = client.ReplClient{
				Source: source,
				Target: target,
			}
			replClientFromHost = client.ReplClient{
				Source: sourceFromHost,
				Target: targetFromHost,
			}
		})
		It("can connect with the provided creds", func() {
			Expect(sourceFromHost.Host).ToNot(BeEmpty())
			db, err := replClient.Connect(client.SOURCE)
			defer client.CloseAndLogError(db)
			Expect(err).ToNot(HaveOccurred())
			db, err = replClient.Connect(client.TARGET)
			defer client.CloseAndLogError(db)
			Expect(err).ToNot(HaveOccurred())
		})

		It("can configure replication on the target", func() {
			// the source should use the "container IP" for this test,
			// else the replica will try to connect to localhost:<dynPort> and fail...
			replClient.Source = source
			db, err := replClient.Connect(client.TARGET)
			defer client.CloseAndLogError(db)
			Expect(err).ToNot(HaveOccurred())
			Expect(replClient.Configure(db)).To(Succeed())
			Expect(db.Close()).To(Succeed())
		})
		It("gets the replication state", func() {
			db, err := replClient.Connect(client.TARGET)
			defer client.CloseAndLogError(db)
			Expect(err).ToNot(HaveOccurred())
			state, err := replClient.CheckReplication(db)

			Expect(err).ToNot(HaveOccurred())
			Expect(state).ToNot(Equal(client.ReplState{}))
			Expect(db.Close()).To(Succeed())
			Expect(state.IORunning).To(Equal("IO thread is running"))
			Expect(state.SQLRunning).To(Equal("SQL thread is running"))
			log.Default().Printf("%v", state.Misc)
		})
	})
})
