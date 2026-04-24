package bpm_client_test

import (
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry-incubator/galera-healthcheck/bpm_client"
	"github.com/cloudfoundry-incubator/galera-healthcheck/bpm_client/bpm_clientfakes"
	"github.com/cloudfoundry-incubator/galera-healthcheck/node_manager"
)

// Compile-time interface compliance checks
var _ node_manager.ProcessClient = (*bpm_client.BpmClient)(nil)
var _ node_manager.MonitClient = (*bpm_client.BpmClient)(nil)

var _ = Describe("BpmClient", func() {
	var (
		client        *bpm_client.BpmClient
		fakeRunner    *bpm_clientfakes.FakeCommandRunner
		bpmBinary     string
		jobName       string
		processName   string
		timeout       time.Duration
	)

	BeforeEach(func() {
		fakeRunner = &bpm_clientfakes.FakeCommandRunner{}
		bpmBinary = "/var/vcap/jobs/bpm/bin/bpm"
		jobName = "pxc-mysql"
		processName = "galera-init"
		timeout = 30 * time.Second
		client = bpm_client.NewClient(bpmBinary, jobName, processName, timeout, fakeRunner)
	})

	Describe("Start", func() {
		Context("when bpm start succeeds", func() {
		BeforeEach(func() {
			// First call (start) returns success, subsequent calls (pid) return running PID
			fakeRunner.RunReturnsOnCall(0, []byte(""), nil)            // bpm start
			fakeRunner.RunReturnsOnCall(1, []byte("12345"), nil)       // bpm pid (running)
		})

		It("executes bpm start command and waits for running status", func() {
			err := client.Start("service-name")

			Expect(err).NotTo(HaveOccurred())
			Expect(fakeRunner.RunCallCount()).To(BeNumerically(">=", 2)) // start + at least one status check
			
			// First call should be start
			command, args := fakeRunner.RunArgsForCall(0)
			Expect(command).To(Equal(bpmBinary))
			Expect(args).To(Equal([]string{"start", jobName, "-p", processName}))
			
			// Second call should be pid check
			command, args = fakeRunner.RunArgsForCall(1)
			Expect(command).To(Equal(bpmBinary))
			Expect(args).To(Equal([]string{"pid", jobName, "-p", processName}))
		})

			It("ignores serviceName parameter and uses configured job/process names", func() {
				err := client.Start("ignored-service-name")

				Expect(err).NotTo(HaveOccurred())
				// Just check the first call (start command) uses correct names
				command, args := fakeRunner.RunArgsForCall(0)
				Expect(command).To(Equal(bpmBinary))
				Expect(args).To(Equal([]string{"start", jobName, "-p", processName}))
			})
		})

		Context("when bmp start fails", func() {
			BeforeEach(func() {
				fakeRunner.RunReturns(nil, errors.New("bmp start failed"))
			})

			It("returns an error", func() {
				err := client.Start("service-name")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("bmp start failed"))
			})
		})
	})

	Describe("Stop", func() {
		Context("when bpm stop succeeds", func() {
			BeforeEach(func() {
				// First call (stop) returns success, subsequent calls (pid) return stopped (non-zero exit)
				fakeRunner.RunReturnsOnCall(0, []byte(""), nil)                           // bpm stop
				fakeRunner.RunReturnsOnCall(1, []byte(""), errors.New("exit status 1"))  // bpm pid (stopped)
			})

			It("executes bpm stop command and waits for stopped status", func() {
				err := client.Stop("service-name")

				Expect(err).NotTo(HaveOccurred())
				Expect(fakeRunner.RunCallCount()).To(BeNumerically(">=", 2)) // stop + at least one status check
				
				// First call should be stop
				command, args := fakeRunner.RunArgsForCall(0)
				Expect(command).To(Equal(bpmBinary))
				Expect(args).To(Equal([]string{"stop", jobName, "-p", processName}))
				
				// Second call should be pid check
				command, args = fakeRunner.RunArgsForCall(1)
				Expect(command).To(Equal(bpmBinary))
				Expect(args).To(Equal([]string{"pid", jobName, "-p", processName}))
			})
		})

		Context("when bmp stop fails", func() {
			BeforeEach(func() {
				fakeRunner.RunReturns(nil, errors.New("bmp stop failed"))
			})

			It("returns an error", func() {
				err := client.Stop("service-name")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("bmp stop failed"))
			})
		})
	})

	Describe("Status", func() {
		Context("when bmp pid succeeds", func() {
			BeforeEach(func() {
				fakeRunner.RunReturns([]byte("12345"), nil)
			})

			It("executes bmp pid command and returns running status", func() {
				status, err := client.Status("service-name")

				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal("running"))
				
				command, args := fakeRunner.RunArgsForCall(0)
				Expect(command).To(Equal(bpmBinary))
				Expect(args).To(Equal([]string{"pid", jobName, "-p", processName}))
			})
		})

		Context("when bpm pid returns non-zero exit code", func() {
			BeforeEach(func() {
				fakeRunner.RunReturns(nil, errors.New("exit status 1"))
			})

			It("returns stopped status to match monit behavior", func() {
				status, err := client.Status("service-name")

				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal("stopped"))
			})
		})

		Context("when bpm command execution fails", func() {
			BeforeEach(func() {
				fakeRunner.RunReturns(nil, errors.New("command not found"))
			})

			It("returns error status", func() {
				status, err := client.Status("service-name")

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("command not found"))
				Expect(status).To(Equal(""))
			})
		})

		Context("when bpm pid returns empty output", func() {
			BeforeEach(func() {
				fakeRunner.RunReturns([]byte(""), nil)
			})

			It("returns stopped status", func() {
				status, err := client.Status("service-name")

				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal("stopped"))
			})
		})

		Context("when bpm pid returns only whitespace", func() {
			BeforeEach(func() {
				fakeRunner.RunReturns([]byte("   \n  "), nil)
			})

			It("returns stopped status", func() {
				status, err := client.Status("service-name")

				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal("stopped"))
			})
		})

		Context("when bpm pid returns valid PID with whitespace", func() {
			BeforeEach(func() {
				fakeRunner.RunReturns([]byte("  12345\n"), nil)
			})

			It("returns running status", func() {
				status, err := client.Status("service-name")

				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal("running"))
			})
		})

		Context("status string compatibility", func() {
			It("only returns running or stopped status (BPM limitation)", func() {
				// BPM only provides binary running/not-running information via 'bmp pid'
				// Unlike monit, we cannot determine "pending", "initializing", or "failing" states
				
				// Test running state
				fakeRunner.RunReturns([]byte("12345"), nil)
				status, err := client.Status("service-name")
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal("running"))
				
				// Test stopped state
				fakeRunner.RunReturns(nil, errors.New("exit status 1"))
				status, err = client.Status("service-name")
				Expect(err).NotTo(HaveOccurred())
				Expect(status).To(Equal("stopped"))
			})
		})
	})
})