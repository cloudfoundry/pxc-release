package node_starter_test

import (
	"errors"
	"os/exec"

	health_checker_fakes "github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker/fakes"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	db_helper_fakes "github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/fakes"
	os_fakes "github.com/cloudfoundry/mariadb_ctrl/os_helper/fakes"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_starter"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Starter", func() {
	var starter node_starter.Starter

	var testLogger *lagertest.TestLogger
	var fakeOs *os_fakes.FakeOsHelper
	var fakeClusterHealthChecker *health_checker_fakes.FakeClusterHealthChecker
	var fakeDBHelper *db_helper_fakes.FakeDBHelper
	var fakeCommandBootstrapStr string
	var fakeCommandBootstrap *exec.Cmd
	var fakeCommandJoinStr string
	var fakeCommandJoin *exec.Cmd

	const databaseStartupTimeout = 10

	ensureManageReadOnlyUser := func() {
		Expect(fakeDBHelper.ManageReadOnlyUserCallCount()).To(Equal(1))
	}

	ensureSeedDatabases := func() {
		Expect(fakeDBHelper.SeedCallCount()).To(BeNumerically(">=", 1))
	}

	ensureBootstrap := func() {
		Expect(fakeDBHelper.StartMysqlInBootstrapCallCount()).To(Equal(1))
	}

	ensureJoin := func() {
		Expect(fakeDBHelper.StartMysqlInJoinCallCount()).To(Equal(1))
	}

	ensureMysqlCmdMatches := func(cmd string) {
		runCmd, err := starter.GetMysqlCmd()
		Expect(err).ToNot(HaveOccurred())
		Expect(runCmd.Path).To(Equal(cmd))
	}

	BeforeEach(func() {
		testLogger = lagertest.NewTestLogger("start_manager")
		fakeOs = new(os_fakes.FakeOsHelper)
		fakeClusterHealthChecker = new(health_checker_fakes.FakeClusterHealthChecker)
		fakeDBHelper = new(db_helper_fakes.FakeDBHelper)
		fakeDBHelper.IsDatabaseReachableReturns(true)

		starter = node_starter.New(
			fakeDBHelper,
			fakeOs,
			config.StartManager{
				DatabaseStartupTimeout: databaseStartupTimeout,
			},
			testLogger,
			fakeClusterHealthChecker,
		)
	})

	Describe("StartNodeFromState", func() {
		BeforeEach(func() {
			fakeCommandBootstrapStr = "fake-command-bootstrap"
			fakeCommandBootstrap = exec.Command(fakeCommandBootstrapStr)
			fakeDBHelper.StartMysqlInBootstrapReturns(fakeCommandBootstrap, nil)
			fakeCommandJoinStr = "fake-command-join"
			fakeCommandJoin = exec.Command(fakeCommandJoinStr)
			fakeDBHelper.StartMysqlInJoinReturns(fakeCommandJoin, nil)
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
				ensureMysqlCmdMatches(fakeCommandJoinStr)
			})
		})

		Context("When mysql starts in less than configured DatabaseStartupTimeout", func() {
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

			It("retries pinging the database until it is reachable", func() {
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
					fakeDBHelper.StartMysqlInBootstrapReturns(nil, errors.New("some errors"))
					fakeDBHelper.StartMysqlInJoinReturns(nil, errors.New("some errors"))
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

			Context("When mysql does not start in less than configured DatabaseStartupTimeout", func() {
				var maxRetryAttempts int

				BeforeEach(func() {
					maxRetryAttempts = databaseStartupTimeout / node_starter.StartupPollingFrequencyInSeconds
					fakeDBHelper.IsDatabaseReachableReturns(false)
				})

				It("returns a timeout error", func() {
					_, err := starter.StartNodeFromState("SINGLE_NODE")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Timeout"))
					Expect(fakeDBHelper.IsDatabaseReachableCallCount()).To(Equal(maxRetryAttempts))
				})

				It("does not attempt to seed the database", func() {
					_, err := starter.StartNodeFromState("SINGLE_NODE")
					Expect(err).To(HaveOccurred())
					Expect(fakeDBHelper.SeedCallCount()).To(Equal(0))
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
		})
	})
})
