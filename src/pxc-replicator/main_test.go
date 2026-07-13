package main_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/cloudfoundry/pxc-release/replicator/client"
	"github.com/cloudfoundry/pxc-release/replicator/config"
	"github.com/cloudfoundry/pxc-release/replicator/testhelper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/testcontainers/testcontainers-go"
	"go.yaml.in/yaml/v3"
)

var _ = Describe("Main", Ordered, func() {
	var sourceName string
	var logBuffer *gbytes.Buffer
	replUser := "replUser"
	var rep, sourceContainer, targetContainer *testcontainers.DockerContainer
	_ = BeforeAll(func() {
		net := testhelper.CreateTestNetwork()
		var source, target config.Target
		source, _, sourceContainer = testhelper.StartPXCInstance(testhelper.GeneratePassword(), "8.4", testhelper.VerifyCA, []string{"source"}, net)
		target, _, targetContainer = testhelper.StartPXCInstance(testhelper.GeneratePassword(), "8.4", testhelper.VerifyCA, []string{"target"}, net)
		sourceName = source.Name
		ctx := context.Background()
		fileReader, err := sourceContainer.CopyFileFromContainer(ctx, "/certs/server-ca.pem")
		Expect(err).ToNot(HaveOccurred())
		fileContents, err := io.ReadAll(fileReader)
		Expect(err).ToNot(HaveOccurred())
		f, err := os.CreateTemp("", "")
		Expect(fileContents).ToNot(BeEmpty())
		Expect(err).ToNot(HaveOccurred())
		_, err = f.Write(fileContents)
		Expect(err).ToNot(HaveOccurred())
		// TODO
		// this file would normally be written by the replicator. For now it's expected to run on the same host as the replica mysqld
		// the code will write it into 'datadir'. it needs to be accessible to the bpm process of mysqld
		err = targetContainer.CopyFileToContainer(ctx, f.Name(), fmt.Sprintf("/tmp/%s.ca.pem", source.Name), 644)
		Expect(err).ToNot(HaveOccurred())
		// targetContainer.CopyFileToContainer(
		source.Creds.Username = replUser
		source.Creds.Password = testhelper.GeneratePassword()

		repClient := &client.ReplClient{
			Source:  source,
			Target:  target,
			DataDir: "/tmp/data",
			DumpDir: "/tmp/dump",
			BinPath: "",
		}

		config, err := yaml.Marshal(repClient)
		log.Printf("%s", string(config))
		Expect(err).ToNot(HaveOccurred())
		logBuffer = gbytes.NewBuffer()
		rep = testhelper.StartReplicatorInContainer("8.4", config, net, logBuffer)
	})
	It("works", func() {
		Eventually(logBuffer, 180).Should(gbytes.Say("Parsed config"))
		Eventually(logBuffer, 180).Should(gbytes.Say("setting up replica"))
		Eventually(logBuffer, 180).Should(gbytes.Say("source version is: 8.4"))
		Eventually(logBuffer, 180).Should(gbytes.Say("target version is: 8.4"))
		Eventually(logBuffer, 180).Should(gbytes.Say("running initial sync as there is no current replication setup"))
		Eventually(logBuffer, 180).Should(gbytes.Say("no matching backup found"))
		Eventually(logBuffer, 180).Should(gbytes.Say("will save dump"))
		Eventually(logBuffer, 180).Should(gbytes.Say("finished backup"))
		Eventually(logBuffer, 180).Should(gbytes.Say("starting restore"))
		Eventually(logBuffer, 180).Should(gbytes.Say("importing dump"))
		Eventually(logBuffer, 180).Should(gbytes.Say(fmt.Sprintf("Source_SSL_Allowed:Yes Source_SSL_CA_File:/tmp/%s.ca.pem.*%s", sourceName, fmt.Sprintf("Source_User:%s", replUser))))
		Eventually(logBuffer, 180).Should(gbytes.Say("IORunning: Yes, SQLRunning: Yes, SQLDelay: 0, SecondsBehind: 0"))
		Expect(rep.Terminate(context.Background())).To(Succeed())
		testcontainers.CleanupContainer(GinkgoTB(), rep, testcontainers.StopTimeout(120*time.Second))
		testcontainers.CleanupContainer(GinkgoTB(), sourceContainer, testcontainers.StopTimeout(120*time.Second))
		testcontainers.CleanupContainer(GinkgoTB(), targetContainer, testcontainers.StopTimeout(120*time.Second))
	})
})
