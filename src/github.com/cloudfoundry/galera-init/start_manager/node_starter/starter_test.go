package node_starter_test

import (
	"errors"
	"os"
	"os/exec"

	"code.cloudfoundry.org/lager/v3/lagertest"

	"github.com/cloudfoundry/galera-init/cluster_health_checker/cluster_health_checkerfakes"
	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/db_helper/db_helperfakes"
	"github.com/cloudfoundry/galera-init/os_helper/os_helperfakes"
	"github.com/cloudfoundry/galera-init/start_manager/node_starter"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Starter", func() {
	var starter node_starter.Starter

	var testLogger *lagertest.TestLogger
	var fakeOs *os_helperfakes.FakeOsHelper
	var fakeClusterHealthChecker *cluster_health_checkerfakes.FakeClusterHealthChecker
	var fakeDBHelper *db_helperfakes.FakeDBHelper
	var fakeCommandBootstrapStr string
	var fakeCommandBootstrap *exec.Cmd
	var fakeCommandJoinStr string
	var fakeCommandJoin *exec.Cmd
	var errorChan chan error
	var grastateFile *os.File

	ensureBootstrap := func() {
		Expect(fakeDBHelper.StartMysqldInBootstrapCallCount()).To(Equal(1))
	}

	ensureJoin := func() {
		Expect(fakeDBHelper.StartMysqldInJoinCallCount()).To(Equal(1))
	}

	ensureMysqlCmdMatches := func(cmd string) {
		runCmd := starter.GetMysqlCmd()
		Expect(runCmd.Path).To(Equal(cmd))
	}

	BeforeEach(func() {
		testLogger = lagertest.NewTestLogger("start_manager")
		fakeOs = new(os_helperfakes.FakeOsHelper)
		errorChan = make(chan error, 1)
		fakeOs.WaitForCommandReturns(errorChan)
		fakeClusterHealthChecker = new(cluster_health_checkerfakes.FakeClusterHealthChecker)
		fakeDBHelper = new(db_helperfakes.FakeDBHelper)
		fakeDBHelper.IsDatabaseReachableReturns(true)

		grastateFile, _ = os.CreateTemp(os.TempDir(), "grastateFile")
		starter = node_starter.NewStarter(
			fakeDBHelper,
			fakeOs,
			config.StartManager{
				GrastateFileLocation: grastateFile.Name(),
			},
			testLogger,
			fakeClusterHealthChecker,
		)
	})

	AfterEach(func() {
		os.Remove(grastateFile.Name())
	})

	Describe("StartNodeFromState", func() {
		BeforeEach(func() {
			fakeCommandBootstrapStr = "fake-command-bootstrap"
			fakeCommandBootstrap = exec.Command(fakeCommandBootstrapStr)
			fakeDBHelper.StartMysqldInBootstrapReturns(fakeCommandBootstrap, nil)
			fakeCommandJoinStr = "fake-command-join"
			fakeCommandJoin = exec.Command(fakeCommandJoinStr)
			fakeDBHelper.StartMysqldInJoinReturns(fakeCommandJoin, nil)
		})

		Context("starting with state SINGLE_NODE", func() {
			BeforeEach(func() {
				fakeClusterHealthChecker.HealthyClusterReturns(false)
			})

			It("bootstraps, seeds databases and sets read only user", func() {
				newNodeState, mysqlErrChan, err := starter.StartNodeFromState("SINGLE_NODE")
				Expect(err).ToNot(HaveOccurred())
				Expect(newNodeState).To(Equal("SINGLE_NODE"))
				Expect(mysqlErrChan).NotTo(BeNil())
				ensureBootstrap()
				ensureMysqlCmdMatches(fakeCommandBootstrapStr)
			})

			Describe("grastate file", func() {
				BeforeEach(func() {
					grastateFile.Chmod(0777)
					err := os.WriteFile(grastateFile.Name(), []byte("IMPORTANT OTHER STUFF\nsafe_to_bootstrap: 0\nLESS IMPORTANT STUFF"), 0777)
					Expect(err).ToNot(HaveOccurred())
				})

				It("updates the grastate file's safe_to_bootstrap", func() {
					_, _, err := starter.StartNodeFromState("SINGLE_NODE")
					Expect(err).ToNot(HaveOccurred())

					grastateFileOutput, _ := os.ReadFile(grastateFile.Name())
					Expect(string(grastateFileOutput)).To(Equal("IMPORTANT OTHER STUFF\nsafe_to_bootstrap: 1\nLESS IMPORTANT STUFF"))
				})

				Describe("when it is not present", func() {
					BeforeEach(func() {
						os.Remove(grastateFile.Name())
					})

					It("does not create the file", func() {
						_, _, err := starter.StartNodeFromState("SINGLE_NODE")
						Expect(err).ToNot(HaveOccurred())
						Expect(grastateFile.Name()).ShouldNot(BeAnExistingFile())
					})
				})
			})
		})

		Context("starting with state NEEDS_BOOTSTRAP", func() {
			Context("when the cluster is not healthy", func() {
				BeforeEach(func() {
					fakeClusterHealthChecker.HealthyClusterReturns(false)
				})

				It("bootstraps, seeds databases and sets read only user", func() {
					newNodeState, _, err := starter.StartNodeFromState("NEEDS_BOOTSTRAP")
					Expect(err).ToNot(HaveOccurred())
					Expect(newNodeState).To(Equal("CLUSTERED"))
					ensureBootstrap()
					ensureMysqlCmdMatches(fakeCommandBootstrapStr)
				})

				Describe("grastate file", func() {
					BeforeEach(func() {
						grastateFile.Chmod(0777)
						err := os.WriteFile(grastateFile.Name(), []byte("IMPORTANT OTHER STUFF\nsafe_to_bootstrap: 0\nLESS IMPORTANT STUFF"), 0777)
						Expect(err).ToNot(HaveOccurred())
					})

					It("updates the grastate file's safe_to_bootstrap", func() {
						_, _, err := starter.StartNodeFromState("NEEDS_BOOTSTRAP")
						Expect(err).ToNot(HaveOccurred())

						grastateFileOutput, _ := os.ReadFile(grastateFile.Name())
						Expect(string(grastateFileOutput)).To(Equal("IMPORTANT OTHER STUFF\nsafe_to_bootstrap: 1\nLESS IMPORTANT STUFF"))
					})

					Describe("when it is not present", func() {
						BeforeEach(func() {
							os.Remove(grastateFile.Name())
						})

						It("does not create the file", func() {
							_, _, err := starter.StartNodeFromState("NEEDS_BOOTSTRAP")
							Expect(err).ToNot(HaveOccurred())
							Expect(grastateFile.Name()).ShouldNot(BeAnExistingFile())
						})
					})
				})
			})

			Context("when the cluster is healthy", func() {
				BeforeEach(func() {
					fakeClusterHealthChecker.HealthyClusterReturns(true)
				})

				It("joins the cluster", func() {
					newNodeState, _, err := starter.StartNodeFromState("NEEDS_BOOTSTRAP")
					Expect(err).ToNot(HaveOccurred())
					Expect(newNodeState).To(Equal("CLUSTERED"))
					ensureJoin()
					ensureMysqlCmdMatches(fakeCommandJoinStr)
				})
			})
		})

		Context("starting with state CLUSTERED", func() {
			BeforeEach(func() {
				fakeClusterHealthChecker.HealthyClusterReturns(false)
			})

			It("joins the cluster", func() {
				newNodeState, _, err := starter.StartNodeFromState("CLUSTERED")
				Expect(err).ToNot(HaveOccurred())
				Expect(newNodeState).To(Equal("CLUSTERED"))
				ensureJoin()
				ensureMysqlCmdMatches(fakeCommandJoinStr)
			})
		})

		Context("starting with state HALTED", func() {
			var fakeCommandHalted *exec.Cmd
			var fakeCommandHaltedStr string

			BeforeEach(func() {
				fakeCommandHaltedStr = "fake-command-halted"
				fakeCommandHalted = exec.Command(fakeCommandHaltedStr)
				fakeOs.StartCommandReturns(fakeCommandHalted, nil)
				fakeOs.RunCommandReturns("WSREP: Recovered position: 00000000-0000-0000-0000-000000000000:123", nil)
			})

			It("starts mysqld with sequence recovery", func() {
				newNodeState, mysqlErrChan, err := starter.StartNodeFromState("HALTED")
				Expect(err).ToNot(HaveOccurred())
				Expect(newNodeState).To(Equal("CLUSTERED"))
				Expect(mysqlErrChan).NotTo(BeNil())
				
				// Verify that StartCommand was called for mysqld with recovery
				Expect(fakeOs.StartCommandCallCount()).To(Equal(1))
				logFile, command, args := fakeOs.StartCommandArgsForCall(0)
				Expect(command).To(Equal("mysqld"))
				Expect(args).To(ContainElement("--defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf"))
				Expect(args).To(ContainElement("--wsrep-start-position=00000000-0000-0000-0000-000000000000:123"))
				
				ensureMysqlCmdMatches(fakeCommandHaltedStr)
			})

			Context("when sequence recovery fails", func() {
				BeforeEach(func() {
					fakeOs.RunCommandReturns("", errors.New("recovery failed"))
				})

				It("returns an error", func() {
					_, _, err := starter.StartNodeFromState("HALTED")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("recovery failed"))
				})
			})

			Context("when sequence number is not found in logs", func() {
				BeforeEach(func() {
					fakeOs.RunCommandReturns("No WSREP recovery info", nil)
				})

				It("returns an error", func() {
					_, _, err := starter.StartNodeFromState("HALTED")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Couldn't find regex"))
				})
			})

			Context("when StartCommand fails", func() {
				BeforeEach(func() {
					fakeOs.StartCommandReturns(nil, errors.New("start command failed"))
				})

				It("returns an error", func() {
					_, _, err := starter.StartNodeFromState("HALTED")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("start command failed"))
				})
			})
		})

		Context("error handling", func() {
			Context("when passed a an invalid state", func() {
				It("forwards the error", func() {
					_, _, err := starter.StartNodeFromState("INVALID_STATE")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Unsupported state file contents"))
				})
			})

			Context("when db exits before starting successfully", func() {
				expectedErr := "Mysqld exited with error; aborting. Review the mysqld error logs for more information."
				It("forwards the error", func() {
					errorChan <- errors.New("db exited")
					fakeDBHelper.IsDatabaseReachableReturns(false)

					var err error
					_, _, err = starter.StartNodeFromState("CLUSTERED")
					Expect(err).Should(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedErr))
				})

				It("forwards an error, even if mysql start exits successfully", func() {
					errorChan <- nil
					fakeDBHelper.IsDatabaseReachableReturns(false)

					var err error
					_, _, err = starter.StartNodeFromState("CLUSTERED")
					Expect(err).Should(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring(expectedErr))
				})
			})

			Context("starting cluster returns an error", func() {
				BeforeEach(func() {
					fakeDBHelper.StartMysqldInBootstrapReturns(nil, errors.New("some errors"))
					fakeDBHelper.StartMysqldInJoinReturns(nil, errors.New("some errors"))
				})

				Context("SINGLE_NODE", func() {
					It("forwards the error", func() {
						_, _, err := starter.StartNodeFromState("SINGLE_NODE")
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("some errors"))
					})
				})

				Context("NEEDS_BOOTSTRAP", func() {
					It("forwards the error", func() {
						_, _, err := starter.StartNodeFromState("NEEDS_BOOTSTRAP")
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("some errors"))
					})
				})

				Context("CLUSTERED", func() {
					It("forwards the error", func() {
						_, _, err := starter.StartNodeFromState("CLUSTERED")
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("some errors"))
					})
				})
			})

		})
	})
})
