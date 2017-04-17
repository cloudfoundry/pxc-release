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

var _ = Describe("PreStarter", func() {
	var prestarter node_starter.Starter

	var testLogger *lagertest.TestLogger
	var fakeOs *os_helperfakes.FakeOsHelper
	var fakeClusterHealthChecker *cluster_health_checkerfakes.FakeClusterHealthChecker
	var fakeDBHelper *mariadb_helperfakes.FakeDBHelper
	var fakeCommandJoinStr string
	var fakeCommandJoin *exec.Cmd
	var errorChan chan error

	ensureJoin := func() {
		Expect(fakeDBHelper.StartMysqldInJoinCallCount()).To(Equal(1))
	}

	ensureNoJoin := func() {
		Expect(fakeDBHelper.StartMysqldInJoinCallCount()).To(Equal(0))
	}

	ensureDatabaseReachableCheck := func() {
		Expect(fakeDBHelper.IsDatabaseReachableCallCount()).ToNot(Equal(0))
	}

	ensureNoDatabaseReachableCheck := func() {
		Expect(fakeDBHelper.IsDatabaseReachableCallCount()).To(Equal(0))
	}

	ensureMysqlCmdMatches := func(cmd string) {
		runCmd, err := prestarter.GetMysqlCmd()
		Expect(err).ToNot(HaveOccurred())
		Expect(runCmd.Path).To(Equal(cmd))
	}

	ensureMysqlCmdNilNoError := func(cmd string) {
		runCmd, err := prestarter.GetMysqlCmd()
		Expect(runCmd).To(BeNil())
		Expect(err).ToNot(HaveOccurred())
	}

	BeforeEach(func() {
		testLogger = lagertest.NewTestLogger("node_prestarter")
		fakeOs = new(os_helperfakes.FakeOsHelper)
		fakeClusterHealthChecker = new(cluster_health_checkerfakes.FakeClusterHealthChecker)
		fakeDBHelper = new(mariadb_helperfakes.FakeDBHelper)
		fakeDBHelper.IsDatabaseReachableReturns(true)
		errorChan = make(chan error, 1)
		fakeOs.WaitForCommandReturns(errorChan)

		prestarter = node_starter.NewPreStarter(
			fakeDBHelper,
			fakeOs,
			config.StartManager{},
			testLogger,
			fakeClusterHealthChecker,
		)
	})

	Describe("StartNodeFromState", func() {
		BeforeEach(func() {
			fakeCommandJoinStr = "fake-command-join"
			fakeCommandJoin = exec.Command(fakeCommandJoinStr)
			fakeDBHelper.StartMysqldInJoinReturns(fakeCommandJoin, nil)
		})

		Context("when mariadb exits before starting successfully", func() {
			expectedErr := "Mysqld exited with error; aborting. Review the mysqld error logs for more information."
			It("forwards the error", func() {
				errorChan <- errors.New("mariadb exited")
				fakeDBHelper.IsDatabaseReachableReturns(false)

				var err error
				_, err = prestarter.StartNodeFromState("CLUSTERED")
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedErr))
			})

			It("forwards an error, even if mysql start exits successfully", func() {
				errorChan <- nil
				fakeDBHelper.IsDatabaseReachableReturns(false)

				var err error
				_, err = prestarter.StartNodeFromState("CLUSTERED")
				Expect(err).Should(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(expectedErr))
			})
		})

		Context("prestarting with state SINGLE_NODE", func() {
			It("does nothing and returns", func() {
				newNodeState, err := prestarter.StartNodeFromState("SINGLE_NODE")
				Expect(err).ToNot(HaveOccurred())
				Expect(newNodeState).To(Equal("SINGLE_NODE"))
				ensureNoJoin()
				ensureMysqlCmdNilNoError(fakeCommandJoinStr)
				ensureNoDatabaseReachableCheck()
			})
		})

		Context("starting with state NEEDS_BOOTSTRAP", func() {
			Context("when the cluster is not healthy", func() {
				BeforeEach(func() {
					fakeClusterHealthChecker.HealthyClusterReturns(false)
				})

				It("does nothing and returns", func() {
					newNodeState, err := prestarter.StartNodeFromState("NEEDS_BOOTSTRAP")
					Expect(err).ToNot(HaveOccurred())
					Expect(newNodeState).To(Equal("NEEDS_BOOTSTRAP"))
					ensureNoJoin()
					ensureMysqlCmdNilNoError(fakeCommandJoinStr)
					ensureNoDatabaseReachableCheck()
				})
			})

			Context("when the cluster is healthy", func() {
				BeforeEach(func() {
					fakeClusterHealthChecker.HealthyClusterReturns(true)
				})

				It("joins the cluster", func() {
					newNodeState, err := prestarter.StartNodeFromState("NEEDS_BOOTSTRAP")
					Expect(err).ToNot(HaveOccurred())
					Expect(newNodeState).To(Equal("CLUSTERED"))
					ensureJoin()
					ensureMysqlCmdMatches(fakeCommandJoinStr)
					ensureDatabaseReachableCheck()
				})
			})
		})

		Context("starting with state CLUSTERED", func() {
			BeforeEach(func() {
				fakeClusterHealthChecker.HealthyClusterReturns(false)
			})

			It("joins the cluster", func() {
				newNodeState, err := prestarter.StartNodeFromState("CLUSTERED")
				Expect(err).ToNot(HaveOccurred())
				Expect(newNodeState).To(Equal("CLUSTERED"))
				ensureJoin()
				ensureMysqlCmdMatches(fakeCommandJoinStr)
				ensureDatabaseReachableCheck()
			})
		})

		Context("When mysqld takes some time to start", func() {
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

			It("retries forever pinging the database until it is reachable", func() {
				_, err := prestarter.StartNodeFromState("CLUSTERED")
				Expect(err).ToNot(HaveOccurred())
				Expect(fakeDBHelper.IsDatabaseReachableCallCount()).To(Equal(expectedRetryAttempts))
			})
		})

		Context("error handling", func() {
			Context("when passed a an invalid state", func() {
				It("forwards the error", func() {
					_, err := prestarter.StartNodeFromState("INVALID_STATE")
					Expect(err).To(HaveOccurred())
					Expect(err.Error()).To(ContainSubstring("Unsupported state file contents"))
					ensureNoJoin()
				})
			})

			Context("starting cluster returns an error", func() {
				BeforeEach(func() {
					fakeDBHelper.StartMysqldInJoinReturns(nil, errors.New("some errors"))
				})

				Context("NEEDS_BOOTSTRAP", func() {
					It("forwards the error", func() {
						fakeClusterHealthChecker.HealthyClusterReturns(true)
						_, err := prestarter.StartNodeFromState("NEEDS_BOOTSTRAP")
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("some errors"))
					})
				})

				Context("CLUSTERED", func() {
					It("forwards the error", func() {
						_, err := prestarter.StartNodeFromState("CLUSTERED")
						Expect(err).To(HaveOccurred())
						Expect(err.Error()).To(ContainSubstring("some errors"))
					})
				})
			})
		})
	})
})
