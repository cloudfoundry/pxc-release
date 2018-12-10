package node_starter_test

import (
	"errors"
	"os/exec"

	"code.cloudfoundry.org/lager/lagertest"
	"github.com/cloudfoundry/galera-init/cluster_health_checker/cluster_health_checkerfakes"
	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/db_helper/db_helperfakes"
	"github.com/cloudfoundry/galera-init/os_helper/os_helperfakes"
	"github.com/cloudfoundry/galera-init/start_manager/node_starter"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"io/ioutil"
	"os"
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

	ensureSeedDatabases := func() {
		Expect(fakeDBHelper.SeedCallCount()).To(BeNumerically(">=", 1))
	}

	ensureBootstrap := func() {
		Expect(fakeDBHelper.StartMysqldInBootstrapCallCount()).To(Equal(1))
	}

	ensureJoin := func() {
		Expect(fakeDBHelper.StartMysqldInJoinCallCount()).To(Equal(1))
	}

	ensureMysqlCmdMatches := func(cmd string) {
		runCmd, err := starter.GetMysqlCmd()
		Expect(err).ToNot(HaveOccurred())
		Expect(runCmd.Path).To(Equal(cmd))
	}

	ensureRunPostStartSQLs := func() {
		Expect(fakeDBHelper.RunPostStartSQLCallCount()).To(BeNumerically(">=", 1))
	}

	ensureTestDatabaseCleanup := func() {
		Expect(fakeDBHelper.TestDatabaseCleanupCallCount()).To(Equal(1))
	}

	BeforeEach(func() {
		testLogger = lagertest.NewTestLogger("start_manager")
		fakeOs = new(os_helperfakes.FakeOsHelper)
		errorChan = make(chan error, 1)
		fakeOs.WaitForCommandReturns(errorChan)
		fakeClusterHealthChecker = new(cluster_health_checkerfakes.FakeClusterHealthChecker)
		fakeDBHelper = new(db_helperfakes.FakeDBHelper)
		fakeDBHelper.IsDatabaseReachableReturns(true)

		grastateFile, _ = ioutil.TempFile(os.TempDir(), "grastateFile")
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
				ensureSeedDatabases()
				ensureRunPostStartSQLs()
				ensureTestDatabaseCleanup()
				ensureMysqlCmdMatches(fakeCommandBootstrapStr)
			})

			Describe("grastate file", func() {
				BeforeEach(func() {
					grastateFile.Chmod(0777)
					err := ioutil.WriteFile(grastateFile.Name(), []byte("IMPORTANT OTHER STUFF\nsafe_to_bootstrap: 0\nLESS IMPORTANT STUFF"), 0777)
					Expect(err).ToNot(HaveOccurred())
				})

				It("updates the grastate file's safe_to_bootstrap", func() {
					_, _, err := starter.StartNodeFromState("SINGLE_NODE")
					Expect(err).ToNot(HaveOccurred())

					grastateFileOutput, _ := ioutil.ReadFile(grastateFile.Name())
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
					ensureSeedDatabases()
					ensureRunPostStartSQLs()
					ensureTestDatabaseCleanup()
					ensureMysqlCmdMatches(fakeCommandBootstrapStr)
				})

				Describe("grastate file", func() {
					BeforeEach(func() {
						grastateFile.Chmod(0777)
						err := ioutil.WriteFile(grastateFile.Name(), []byte("IMPORTANT OTHER STUFF\nsafe_to_bootstrap: 0\nLESS IMPORTANT STUFF"), 0777)
						Expect(err).ToNot(HaveOccurred())
					})

					It("updates the grastate file's safe_to_bootstrap", func() {
						_, _, err := starter.StartNodeFromState("NEEDS_BOOTSTRAP")
						Expect(err).ToNot(HaveOccurred())

						grastateFileOutput, _ := ioutil.ReadFile(grastateFile.Name())
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
					ensureSeedDatabases()
					ensureRunPostStartSQLs()
					ensureTestDatabaseCleanup()
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
				ensureSeedDatabases()
				ensureRunPostStartSQLs()
				ensureTestDatabaseCleanup()
				ensureMysqlCmdMatches(fakeCommandJoinStr)
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

			Context("when database seeding fails", func() {
				var expectedErr error
				BeforeEach(func() {
					expectedErr = errors.New("seeding databases failed")
					fakeDBHelper.SeedReturns(expectedErr)
				})

				It("forwards the error", func() {
					_, _, err := starter.StartNodeFromState("SINGLE_NODE")
					Expect(err).To(HaveOccurred())
					Expect(err).To(Equal(expectedErr))
				})
			})

			Context("when running post start sql fails", func() {
				BeforeEach(func() {
					fakeDBHelper.RunPostStartSQLReturns(errors.New("post start sql failed"))
				})

				It("forwards the error", func() {
					_, _, err := starter.StartNodeFromState("SINGLE_NODE")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("post start sql failed"))
				})
			})

			Context("when running test database cleanup fails", func() {
				BeforeEach(func() {
					fakeDBHelper.TestDatabaseCleanupReturns(errors.New("test database cleanup failed"))
				})

				It("forwards the error", func() {
					_, _, err := starter.StartNodeFromState("SINGLE_NODE")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("test database cleanup failed"))
				})
			})
		})
	})
})
