package main_test

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/cloudfoundry/pxc-release/replicator/client"
	"github.com/cloudfoundry/pxc-release/replicator/testhelper"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/testcontainers/testcontainers-go"
	"go.yaml.in/yaml/v3"
)

var _ = Describe("Main", Ordered, func() {
	var logBuffer *gbytes.Buffer
	var rep *testcontainers.DockerContainer
	_ = BeforeAll(func() {
		net := testhelper.CreateTestNetwork()
		source, _, sourceContainer := testhelper.StartContainerInstance(testhelper.GeneratePassword(), testhelper.GeneratePassword(), "8.4", testhelper.VerifyCA, []string{"source"}, net)
		target, _, targetContainer := testhelper.StartContainerInstance(testhelper.GeneratePassword(), testhelper.GeneratePassword(), "8.4", testhelper.VerifyCA, []string{"target"}, net)

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
		err = targetContainer.CopyFileToContainer(ctx, f.Name(), fmt.Sprintf("%s/source-server-ca.pem", "/tmp"), 644)
		Expect(err).ToNot(HaveOccurred())
		// targetContainer.CopyFileToContainer(
		repClient := client.ReplClient{
			Source:  source,
			Target:  target,
			DataDir: testhelper.DataDir,
			BinDir:  testhelper.DataDir,
		}

		config, err := yaml.Marshal(repClient)
		log.Default().Printf("%s", string(config))
		Expect(err).ToNot(HaveOccurred())
		logBuffer = gbytes.NewBuffer()
		rep = testhelper.StartReplicatorInContainer("8.4", config, net, logBuffer)
	})
	_ = AfterAll(func() {
	})
	It("works", func() {
		Eventually(logBuffer, 180).Should(gbytes.Say("Parsed config"))
		Eventually(logBuffer, 180).Should(gbytes.Say("setting up replica"))
		Eventually(logBuffer, 180).Should(gbytes.Say("source version is:"))
		Eventually(logBuffer, 180).Should(gbytes.Say("target version is:"))
		Eventually(logBuffer, 180).Should(gbytes.Say("will save dump"))
		Eventually(logBuffer, 180).Should(gbytes.Say("finished backup"))
		Eventually(logBuffer, 180).Should(gbytes.Say("replication state:"))
		Eventually(logBuffer, 1800).Should(gbytes.Say("IORunning: Yes, SQLRunning: Yes"))
		defer rep.Terminate(context.Background())
	})
})
