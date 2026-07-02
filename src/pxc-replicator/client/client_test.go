package client_test

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cloudfoundry/pxc-release/replicator/client"
	"github.com/cloudfoundry/pxc-release/replicator/config"
	"github.com/cloudfoundry/pxc-release/replicator/testhelper"
	"github.com/cloudfoundry/pxc-release/replicator/utils"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/testcontainers/testcontainers-go"
)

var _ = Describe("Client/Client", func() {
	var replClient client.ReplClient
	var source, sourceFromHost, _, targetFromHost config.Target

	Describe("using tls connections", Ordered, FlakeAttempts(3), func() {
		_ = BeforeAll(func() {
			testNet := testhelper.CreateTestNetwork()

			source, sourceFromHost, _ = testhelper.StartPXCInstance(testhelper.GeneratePassword(), "8.4", testhelper.VerifyCA, []string{"source"}, testNet)
			sourceFromHost.Creds.AdminUsername, sourceFromHost.Creds.AdminPassword = "", ""
			source.Creds.AdminUsername, source.Creds.AdminPassword = "", ""
			replClient = client.ReplClient{
				Source:  sourceFromHost,
				Target:  config.Target{},
				DataDir: dataDir,
				DumpDir: dataDir,
				BinPath: mysqlBinPath,
			}
			Expect(replClient.InitFiles()).To(Succeed())
		})
		It("will work", func() {
			db, err := replClient.ConnectSource()
			Expect(err).To(BeNil())
			Expect(db.Close()).To(Succeed())
		})
	})

	Describe("mismatched versions", Ordered, FlakeAttempts(3), func() {
		_ = BeforeAll(func() {
			testNet := testhelper.CreateTestNetwork()

			source, sourceFromHost, _ = testhelper.StartPXCInstance(testhelper.GeneratePassword(), "8.0", testhelper.TLSDisabled, []string{"source"}, testNet)
			sourceFromHost.Creds.AdminUsername, sourceFromHost.Creds.AdminPassword = "", ""
			source.Creds.AdminUsername, source.Creds.AdminPassword = "", ""
			_, targetFromHost, _ = testhelper.StartPXCInstance(testhelper.GeneratePassword(), "8.4", testhelper.TLSDisabled, []string{"target"}, testNet)
			replClient = client.ReplClient{
				Source:  sourceFromHost,
				Target:  targetFromHost,
				DumpDir: dataDir,
				DataDir: dataDir,
				BinPath: mysqlBinPath,
			}
			Expect(replClient.InitFiles()).To(Succeed())
		})

		It("will generate an error about mismatched versions", func() {
			source, err := replClient.ConnectSource()
			Expect(err).ToNot(HaveOccurred())
			target, err := replClient.ConnectTarget()
			Expect(err).ToNot(HaveOccurred())
			Expect(replClient.CheckVersion(source, target)).To(MatchError(MatchRegexp("sourceVersion: 8.0 does not match targetVersion: 8.4")))
			Expect(source.Close()).To(Succeed())
			Expect(target.Close()).To(Succeed())
		})
	})
	Describe("it checks for elligible backups", Ordered, FlakeAttempts(3), func() {
		var testNet *testcontainers.DockerNetwork
		_ = BeforeAll(func() {
			dir, err := os.MkdirTemp("", "")
			Expect(err).ToNot(HaveOccurred())

			testNet = testhelper.CreateTestNetwork()
			_, sourceFromHost, _ = testhelper.StartPXCInstance(testhelper.GeneratePassword(), "8.4", testhelper.TLSDisabled, []string{"source"}, testNet)
			_, targetFromHost, _ = testhelper.StartPXCInstance(testhelper.GeneratePassword(), "8.4", testhelper.TLSDisabled, []string{"target"}, testNet)
			replClient = client.ReplClient{
				Target:  targetFromHost,
				Source:  sourceFromHost,
				DataDir: dataDir,
				DumpDir: dir,
				BinPath: mysqlBinPath,
			}
			Expect(replClient.InitFiles()).To(Succeed())
		})
		It("finds no backups when there are none", func() {
			path, err := replClient.FindElligibleBackup()
			Expect(path).To(BeEmpty())
			Expect(err).ToNot(HaveOccurred())
		})
		It("finds elligible backups", func() {
			Expect(replClient.SyncSourceToTarget()).To(Succeed())
			path, err := replClient.FindElligibleBackup()
			Expect(err).ToNot(HaveOccurred())
			Expect(path).ToNot(BeEmpty())
		})
		It("skips taking another backup if it finds a usable one", func() {
			_, targetFromHost, _ = testhelper.StartPXCInstance(testhelper.GeneratePassword(), "8.4", testhelper.TLSDisabled, []string{"target"}, testNet)
			replClient.Target = targetFromHost
			Expect(replClient.InitFiles()).To(Succeed())
			entries, err := os.ReadDir(replClient.DumpDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(1))
			Expect(replClient.SyncSourceToTarget()).To(Succeed())
			entries, err = os.ReadDir(replClient.DumpDir)
			Expect(err).ToNot(HaveOccurred())
			Expect(entries).To(HaveLen(1))
		})
	})
	Describe("checking if replication is enablded", func() {
		_ = BeforeEach(func() {
			testNet := testhelper.CreateTestNetwork()

			_, targetFromHost, _ = testhelper.StartPXCInstance(testhelper.GeneratePassword(), "8.4", testhelper.TLSDisabled, []string{"target"}, testNet)
			replClient = client.ReplClient{
				Target:  targetFromHost,
				DataDir: dataDir,
				DumpDir: dataDir,
				BinPath: mysqlBinPath,
			}
			Expect(replClient.InitFiles()).To(Succeed())
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
	Describe("creating the replica user", Ordered, FlakeAttempts(3), func() {
		_ = BeforeAll(func() {
			testNet := testhelper.CreateTestNetwork()

			source, sourceFromHost, _ = testhelper.StartPXCInstance(testhelper.GeneratePassword(), testhelper.Tag, testhelper.TLSDisabled, []string{"source"}, testNet)
			_, targetFromHost, _ = testhelper.StartPXCInstance(testhelper.GeneratePassword(), testhelper.Tag, testhelper.TLSDisabled, []string{"target"}, testNet)
			sourceFromHost.Creds.Password = ""
			sourceFromHost.Creds.Username = ""

			replClient = client.ReplClient{
				Source:  sourceFromHost,
				Target:  targetFromHost,
				DataDir: dataDir,
				DumpDir: dataDir,
				BinPath: mysqlBinPath,
			}
			Expect(replClient.InitFiles()).To(Succeed())
		})
		It("will not create a user when no creds were provided", func() {
			Expect(replClient.Source.Creds.Password).To(BeEmpty())
			Expect(replClient.Source.Creds.Username).To(BeEmpty())
			_, err := replClient.ConnectSource()
			Expect(err).To(MatchError(MatchRegexp("admin credentials provided but backup user name and password are missing")))
		})
		It("will create a user when creds were provided", func() {
			username := testhelper.GeneratePassword()
			password := testhelper.GeneratePassword()

			adminUser := replClient.Source.Creds.AdminUsername
			adminPass := replClient.Source.Creds.AdminPassword

			replClient.Source.Creds.Username = username
			replClient.Source.Creds.Password = password
			replClient.Source.Creds.AdminUsername = ""
			replClient.Source.Creds.AdminPassword = ""

			// user doesn't exist yet
			_, err := replClient.ConnectSource()
			Expect(err).To(MatchError(MatchRegexp(fmt.Sprintf("Access denied for user '%s'", username))))

			// this will ensure the above user exists
			replClient.Source.Creds.AdminUsername = adminUser
			replClient.Source.Creds.AdminPassword = adminPass
			_, err = replClient.ConnectSource()
			Expect(err).ToNot(HaveOccurred())

			// completely remove admin creds, to prove that the new user exists
			source.Creds.Username = username
			source.Creds.Password = password
			source.Creds.AdminUsername = ""
			source.Creds.AdminPassword = ""

			endToEnd(replClient, source)
		})
	})
	Describe("updated creds through syncing initial state", Ordered, FlakeAttempts(3), func() {
		_ = BeforeAll(func() {
			testNet := testhelper.CreateTestNetwork()

			source, sourceFromHost, _ = testhelper.StartPXCInstance(testhelper.GeneratePassword(), testhelper.Tag, testhelper.TLSDisabled, []string{"source"}, testNet)
			sourceFromHost.Creds.AdminUsername, sourceFromHost.Creds.AdminPassword = "", ""
			source.Creds.AdminUsername, source.Creds.AdminPassword = "", ""
			_, targetFromHost, _ = testhelper.StartPXCInstance(testhelper.GeneratePassword(), testhelper.Tag, testhelper.TLSDisabled, []string{"target"}, testNet)

			replClient = client.ReplClient{
				Source:  sourceFromHost,
				Target:  targetFromHost,
				DataDir: dataDir,
				DumpDir: dataDir,
				BinPath: mysqlBinPath,
			}
			Expect(replClient.InitFiles()).To(Succeed())
		})

		It("will work because the connection is already established", func() {
			endToEnd(replClient, source)
		})
	})
	Describe("full start procedure", Ordered, FlakeAttempts(3), func() {
		_ = BeforeAll(func() {
			testNet := testhelper.CreateTestNetwork()

			source, sourceFromHost, _ = testhelper.StartPXCInstance("test", testhelper.Tag, testhelper.TLSDisabled, []string{"source"}, testNet)
			sourceFromHost.Creds.AdminUsername, sourceFromHost.Creds.AdminPassword = "", ""
			source.Creds.AdminUsername, source.Creds.AdminPassword = "", ""
			_, targetFromHost, _ = testhelper.StartPXCInstance("test", testhelper.Tag, testhelper.TLSDisabled, []string{"target"}, testNet)

			replClient = client.ReplClient{
				Source:  sourceFromHost,
				Target:  targetFromHost,
				DataDir: dataDir,
				DumpDir: dataDir,
				BinPath: mysqlBinPath,
			}
			Expect(replClient.InitFiles()).To(Succeed())
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
	Eventually(func() bool {
		time.Sleep(time.Second)
		state, err = replClient.CheckReplication(db)
		Expect(err).ToNot(HaveOccurred())
		return state.SQLRunning == "Yes" && state.IORunning == "Yes"
	}, time.Minute).Should(BeTrue())
	Expect(err).ToNot(HaveOccurred())
	Expect(db.Close()).To(Succeed())
}
