package node_starter_test

import (
	"errors"
	"os/exec"

	"code.cloudfoundry.org/lager/lagertest"
	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker/cluster_health_checkerfakes"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/mariadb_helperfakes"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper/os_helperfakes"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_starter"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Starter", func() {
	var starter node_starter.Starter

	var testLogger *lagertest.TestLogger
	var fakeOs *os_helperfakes.FakeOsHelper
	var fakeClusterHealthChecker *cluster_health_checkerfakes.FakeClusterHealthChecker
	var fakeDBHelper *mariadb_helperfakes.FakeDBHelper
	var fakeCommandBootstrapStr string
	var fakeCommandBootstrap *exec.Cmd
	var fakeCommandJoinStr string
	var fakeCommandJoin *exec.Cmd

	ensureManageReadOnlyUser := func() {
		Expect(fakeDBHelper.ManageReadOnlyUserCallCount()).To(Equal(1))
	}

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
		fakeClusterHealthChecker = new(cluster_health_checkerfakes.FakeClusterHealthChecker)
		fakeDBHelper = new(mariadb_helperfakes.FakeDBHelper)
		fakeDBHelper.IsDatabaseReachableReturns(true)

		starter = node_starter.NewStarter(
			fakeDBHelper,
			fakeOs,
			config.StartManager{},
			testLogger,
			fakeClusterHealthChecker,
		)
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
				newNodeState, err := starter.StartNodeFromState("SINGLE_NODE")
				Expect(err).ToNot(HaveOccurred())
				Expect(newNodeState).To(Equal("SINGLE_NODE"))
				ensureBootstrap()
				ensureSeedDatabases()
				ensureManageReadOnlyUser()
				ensureRunPostStartSQLs()
				ensureTestDatabaseCleanup()
				ensureMysqlCmdMatches(fakeCommandBootstrapStr)
			})
		})

		Context("starting with state NEEDS_BOOTSTRAP", func() {
			Context("when the cluster is not healthy", func() {
				BeforeEach(func() {
					fakeClusterHealthChecker.HealthyClusterReturns(false)
				})

				It("bootstraps, seeds databases and sets read only user", func() {
					newNodeState, err := starter.StartNodeFromState("NEEDS_BOOTSTRAP")
					Expect(err).ToNot(HaveOccurred())
					Expect(newNodeState).To(Equal("CLUSTERED"))
					ensureBootstrap()
					ensureSeedDatabases()
					ensureManageReadOnlyUser()
					ensureRunPostStartSQLs()
					ensureTestDatabaseCleanup()
					ensureMysqlCmdMatches(fakeCommandBootstrapStr)
				})
			})

			Context("when the cluster is healthy", func() {
				BeforeEach(func() {
					fakeClusterHealthChecker.HealthyClusterReturns(true)
				})

				It("joins the cluster", func() {
					newNodeState, err := starter.StartNodeFromState("NEEDS_BOOTSTRAP")
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
				newNodeState, err := starter.StartNodeFromState("CLUSTERED")
				Expect(err).ToNot(HaveOccurred())
				Expect(newNodeState).To(Equal("CLUSTERED"))
				ensureJoin()
				ensureSeedDatabases()
				ensureManageReadOnlyUser()
				ensureRunPostStartSQLs()
				ensureTestDatabaseCleanup()
				ensureMysqlCmdMatches(fakeCommandJoinStr)
			})
		})

		Context("When mysqld starts in under the startup timeout", func() {
			var expectedRetryAttempts int

			BeforeEach(func() {
				numTries := 0
				expectedRetryAttempts = 2

				fakeDBHelper.IsDatabaseReachableStub = func() bool {
					numTries++
					if numTries < expectedRetryAttempts {
						return false
					} else {
						return true
					}
				}
			})

			It("eventually succeeds", func() {
				_, err := starter.StartNodeFromState("CLUSTERED")
				Expect(err).ToNot(HaveOccurred())
				Expect(fakeDBHelper.IsDatabaseReachableCallCount()).To(Equal(expectedRetryAttempts))
			})
		})

		Context("error handling", func() {
			Context("when passed a an invalid state", func() {
				It("forwards the error", func() {
					_, err := starter.StartNodeFromState("INVALID_STATE")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Unsupported state file contents"))
				})
			})

			Context("starting cluster returns an error", func() {
				BeforeEach(func() {
					fakeDBHelper.StartMysqldInBootstrapReturns(nil, errors.New("some errors"))
					fakeDBHelper.StartMysqldInJoinReturns(nil, errors.New("some errors"))
				})

				Context("SINGLE_NODE", func() {
					It("forwards the error", func() {
						_, err := starter.StartNodeFromState("SINGLE_NODE")
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("some errors"))
					})
				})

				Context("NEEDS_BOOTSTRAP", func() {
					It("forwards the error", func() {
						_, err := starter.StartNodeFromState("NEEDS_BOOTSTRAP")
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("some errors"))
					})
				})

				Context("CLUSTERED", func() {
					It("forwards the error", func() {
						_, err := starter.StartNodeFromState("CLUSTERED")
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
					_, err := starter.StartNodeFromState("SINGLE_NODE")
					Expect(err).To(HaveOccurred())
					Expect(err).To(Equal(expectedErr))
				})
			})

			Context("when creating read only user fails", func() {
				BeforeEach(func() {
					fakeDBHelper.ManageReadOnlyUserReturns(errors.New("some error"))
				})

				It("forwards the error", func() {
					_, err := starter.StartNodeFromState("SINGLE_NODE")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("some error"))
				})
			})

			Context("when running post start sql fails", func() {
				BeforeEach(func() {
					fakeDBHelper.RunPostStartSQLReturns(errors.New("post start sql failed"))
				})

				It("forwards the error", func() {
					_, err := starter.StartNodeFromState("SINGLE_NODE")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("post start sql failed"))
				})
			})

			Context("when running test database cleanup fails", func() {
				BeforeEach(func() {
					fakeDBHelper.TestDatabaseCleanupReturns(errors.New("test database cleanup failed"))
				})

				It("forwards the error", func() {
					_, err := starter.StartNodeFromState("SINGLE_NODE")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("test database cleanup failed"))
				})
			})
		})
	})
})
