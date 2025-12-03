package main_test

import (
	"os/exec"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"

	"github.com/cloudfoundry-incubator/switchboard/internal/testing/docker"
)

var _ = Describe("PingDB", func() {
	Context("setup and launch conditions", func() {
		It("requires a PORT environment variable", func() {
			cmd := exec.Command(pingdbPath)
			cmd.Env = append(cmd.Env, "TIMEOUT=1s")
			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			Eventually(session, "2s").Should(gexec.Exit())
			Expect(session.ExitCode()).NotTo(BeZero())
			Expect(session.Out.Contents()).To(BeEmpty())
			Expect(session.Err).To(gbytes.Say(`Error: bad configuration - env: required environment variable "PORT" is not set`))
		})

		It("requires a TIMEOUT environment variable", func() {
			cmd := exec.Command(pingdbPath)
			cmd.Env = append(cmd.Env, "PORT=3306")
			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			Eventually(session).Should(gexec.Exit())
			Expect(session.ExitCode()).NotTo(BeZero())
			Expect(session.Out.Contents()).To(BeEmpty())
			Expect(session.Err).To(gbytes.Say(`Error: bad configuration - env: required environment variable "TIMEOUT" is not set`))
		})

		It("requires TIMEOUT to be a positive integer", func() {
			cmd := exec.Command(pingdbPath)
			cmd.Env = append(cmd.Env, "TIMEOUT=-1s", "PORT=3306")
			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			Eventually(session).Should(gexec.Exit())
			Expect(session.Out.Contents()).To(BeEmpty())
			Expect(session.Err).To(gbytes.Say(`Error: bad configuration - TIMEOUT must be a positive duration`))
		})

		When("there is no running database at the specified port", func() {
			It("fails and relays the error text", func() {
				cmd := exec.Command(pingdbPath)
				cmd.Env = append(cmd.Env, "TIMEOUT=1s", "PORT=3306")
				session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, time.Second*2).Should(gexec.Exit())
				Expect(session.ExitCode()).NotTo(BeZero())
				Expect(session.Out.Contents()).To(BeEmpty())
				Expect(session.Err).To(gbytes.Say("Error: database ping:.*connection refused"))
			})
		})
	})

	Context("db status", Ordered, func() {
		var (
			containerName string
			exposedPort   string
		)
		BeforeAll(func() {
			containerSpec := docker.ContainerSpec{
				Name:  "pingdb." + uuid.New().String(),
				Image: "percona/percona-xtradb-cluster:8.0",
				Ports: []string{"3306"},
				Env: []string{
					"MYSQL_ALLOW_EMPTY_PASSWORD=yes",
				},
				HealthCmd:      "mysqladmin -u root --host=127.0.0.1 ping",
				HealthInterval: "2s",
			}
			containerName = docker.RunContainer(containerSpec) // includes deferred container cleanup

			Expect(docker.WaitHealthy(containerName, 5*time.Minute)).To(Succeed())

			exposedPort = docker.ContainerPort(containerName, "3306")
		})

		When("the db is running normally", func() {
			It("succeeds with no output", func() {
				cmd := exec.Command(pingdbPath)
				cmd.Env = append(cmd.Env, "TIMEOUT=1s", "PORT="+exposedPort)
				session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, time.Second*2).Should(gexec.Exit())
				Expect(session.ExitCode()).To(BeZero(), "unexpected non-zero exit code")
				Expect(session.Out.Contents()).To(BeEmpty())
				Expect(session.Err.Contents()).To(BeEmpty())
			})
		})

		When("the db is present but non-responsive", func() {
			It("fails and reports a timeout error", func() {
				// Pause our running DB
				err := docker.Kill(containerName, "SIGSTOP")
				Expect(err).NotTo(HaveOccurred())

				cmd := exec.Command(pingdbPath)
				cmd.Env = append(cmd.Env, "TIMEOUT=1s", "PORT="+exposedPort)
				session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
				Expect(err).NotTo(HaveOccurred())

				Eventually(session, time.Second*2).Should(gexec.Exit())
				Expect(session.ExitCode()).To(Equal(1), "unexpected exit code")
				Expect(session.Out.Contents()).To(BeEmpty())
				Expect(session.Err).To(gbytes.Say("Error: database ping timeout after 1s"))

				// Resume our running DB
				err = docker.Kill(containerName, "SIGCONT")
				Expect(err).NotTo(HaveOccurred())
			})
		})
	})

	// An unlikely but possible scenario.
	When("the test user exists and is pingable in the database", func() {
		var (
			containerName string
			exposedPort   string
		)
		BeforeEach(func() {
			containerSpec := docker.ContainerSpec{
				Name:  "pingdb." + uuid.New().String(),
				Image: "percona/percona-xtradb-cluster:8.0",
				Ports: []string{"3306"},
				Env: []string{
					"MYSQL_ALLOW_EMPTY_PASSWORD=yes",
					"MYSQL_USER=pingdb",
					"MYSQL_PASSWORD=pingdb",
				},
				HealthCmd:      "mysqladmin -u root --host=127.0.0.1 ping",
				HealthInterval: "2s",
			}
			containerName = docker.RunContainer(containerSpec)
			docker.WaitHealthy(containerName, 5*time.Minute)
			exposedPort = docker.ContainerPort(containerName, "3306")
		})

		It("pings successfully", func() {
			cmd := exec.Command(pingdbPath)
			cmd.Env = append(cmd.Env, "TIMEOUT=1s", "PORT="+exposedPort)
			session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())

			Eventually(session, time.Second*2).Should(gexec.Exit())
			Expect(session.ExitCode()).To(BeZero(), "unexpected non-zero exit code")
			Expect(session.Out.Contents()).To(BeEmpty())
			Expect(session.Err.Contents()).To(BeEmpty())
		})
	})
})
