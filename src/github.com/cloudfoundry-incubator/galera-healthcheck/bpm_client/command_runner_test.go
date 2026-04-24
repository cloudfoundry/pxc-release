package bpm_client_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/galera-healthcheck/bpm_client"
)

var _ = Describe("RealCommandRunner", func() {
	var runner *bpm_client.RealCommandRunner

	BeforeEach(func() {
		runner = bpm_client.NewRealCommandRunner(5 * time.Second)
	})

	Describe("Run", func() {
		Context("when command succeeds", func() {
			It("returns output", func() {
				output, err := runner.Run("echo", "test")

				Expect(err).NotTo(HaveOccurred())
				Expect(string(output)).To(Equal("test\n"))
			})
		})

		Context("when command fails", func() {
			It("returns error", func() {
				_, err := runner.Run("false")

				Expect(err).To(HaveOccurred())
			})
		})

		Context("when command times out", func() {
			It("returns timeout error", func() {
				shortRunner := bpm_client.NewRealCommandRunner(100 * time.Millisecond)
				_, err := shortRunner.Run("sleep", "1")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("killed"))
			})
		})
	})
})

var _ = Describe("BpmClient New constructor", func() {
	It("creates client with real command runner", func() {
		client := bpm_client.New("/usr/bin/bpm", "test-job", "test-process", 30*time.Second)

		Expect(client).NotTo(BeNil())
		Expect(client.BpmBinary).To(Equal("/usr/bin/bpm"))
		Expect(client.JobName).To(Equal("test-job"))
		Expect(client.ProcessName).To(Equal("test-process"))
		Expect(client.Timeout).To(Equal(30 * time.Second))
	})
})