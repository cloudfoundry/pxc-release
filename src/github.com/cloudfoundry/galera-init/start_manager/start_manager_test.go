package start_manager_test

import (
	"errors"
	"fmt"

	health_checker_fakes "github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker/fakes"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	db_helper_fakes "github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/fakes"
	os_fakes "github.com/cloudfoundry/mariadb_ctrl/os_helper/fakes"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_starter"
	upgrader_fakes "github.com/cloudfoundry/mariadb_ctrl/upgrader/fakes"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/cloudfoundry/mariadb_ctrl/start_manager"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("StartManager", func() {

	var mgr StartManager

	var testLogger *lagertest.TestLogger
	var fakeOs *os_fakes.FakeOsHelper
	var fakeClusterHealthChecker *health_checker_fakes.FakeClusterHealthChecker
	var fakeUpgrader *upgrader_fakes.FakeUpgrader
	var fakeDBHelper *db_helper_fakes.FakeDBHelper

	const stateFileLocation = "/stateFileLocation"
	const databaseStartupTimeout = 10

	type managerArgs struct {
		NodeIndex int
		NodeCount int
	}

	ensureSeedDatabases := func() {
		Expect(fakeDBHelper.SeedCallCount()).To(BeNumerically(">=", 1))
	}

	ensureStateFileContentIs := func(expected string) {
		count := fakeOs.WriteStringToFileCallCount()
		filename, contents := fakeOs.WriteStringToFileArgsForCall(count - 1)
		Expect(filename).To(Equal(stateFileLocation))
		Expect(contents).To(Equal(expected))
	}

	ensureNoWriteToStateFile := func() {
		count := fakeOs.WriteStringToFileCallCount()
		Expect(count).To(Equal(0))
	}

	ensureBootstrapWithStateFileContents := func(contents string) {
		Expect(fakeDBHelper.StartMysqlInBootstrapCallCount()).To(Equal(1))
		ensureStateFileContentIs(contents)
	}

	ensureJoin := func() {
		Expect(fakeDBHelper.StartMysqlInJoinCallCount()).To(Equal(1))
		ensureStateFileContentIs(node_starter.Clustered)
	}

	createManager := func(args managerArgs) StartManager {

		clusterIps := []string{}
		for i := 0; i < args.NodeCount; i++ {
			clusterIps = append(clusterIps, fmt.Sprintf("0.0.0.%d", i+1))
		}

		return New(
			fakeOs,
			config.StartManager{
				StateFileLocation:      stateFileLocation,
				MyIP:                   clusterIps[args.NodeIndex],
				ClusterIps:             clusterIps,
				DatabaseStartupTimeout: databaseStartupTimeout,
			},
			fakeDBHelper,
			fakeUpgrader,
			testLogger,
			fakeClusterHealthChecker,
		)
	}

	BeforeEach(func() {
		testLogger = lagertest.NewTestLogger("start_manager")
		fakeOs = new(os_fakes.FakeOsHelper)
		fakeClusterHealthChecker = new(health_checker_fakes.FakeClusterHealthChecker)
		fakeUpgrader = new(upgrader_fakes.FakeUpgrader)
		fakeDBHelper = new(db_helper_fakes.FakeDBHelper)

		fakeDBHelper.IsProcessRunningReturns(false)
		fakeDBHelper.IsDatabaseReachableReturns(true)
	})

	Context("When a mysql process is already running", func() {
		BeforeEach(func() {
			mgr = createManager(managerArgs{
				NodeCount: 3,
			})
			fakeDBHelper.IsProcessRunningReturns(true)
		})

		It("kills the process before continuing", func() {
			err := mgr.Execute()
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeDBHelper.StopMysqlCallCount()).To(Equal(1))
		})
	})

	Context("When StartMysqlInBootstrap exits with an error", func() {
		BeforeEach(func() {
			mgr = createManager(managerArgs{
				NodeCount: 3,
			})
			fakeDBHelper.StartMysqlInBootstrapReturns(nil, errors.New("some errors"))
		})
		It("forwards the error", func() {
			err := mgr.Execute()
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When StartMysqlInJoin exits with an error", func() {
		BeforeEach(func() {
			mgr = createManager(managerArgs{
				NodeIndex: 1,
				NodeCount: 3,
			})
			fakeDBHelper.StartMysqlInJoinReturns(nil, errors.New("some errors"))
		})
		It("forwards the error", func() {
			err := mgr.Execute()
			Expect(err).To(HaveOccurred())
		})
	})

	Context("When mysql starts in less than configured DatabaseStartupTimeout", func() {
		var expectedRetryAttempts int

		BeforeEach(func() {
			mgr = createManager(managerArgs{
				NodeIndex: 1,
				NodeCount: 3,
			})

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
			err := mgr.Execute()
			Expect(err).ToNot(HaveOccurred())
			Expect(fakeDBHelper.IsDatabaseReachableCallCount()).To(Equal(expectedRetryAttempts))
		})
	})

	Context("When mysql does not start in less than configured DatabaseStartupTimeout", func() {
		var maxRetryAttempts int

		BeforeEach(func() {
			mgr = createManager(managerArgs{
				NodeIndex: 1,
				NodeCount: 3,
			})

			maxRetryAttempts = databaseStartupTimeout / node_starter.StartupPollingFrequencyInSeconds
			fakeDBHelper.IsDatabaseReachableReturns(false)
		})

		It("returns a timeout error", func() {
			err := mgr.Execute()
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("Timeout"))
			Expect(fakeDBHelper.IsDatabaseReachableCallCount()).To(Equal(maxRetryAttempts))
		})

		It("does not attempt to seed the database", func() {
			err := mgr.Execute()
			Expect(err).To(HaveOccurred())
			Expect(fakeDBHelper.SeedCallCount()).To(Equal(0))
		})

		It("does not write to the state file", func() {
			err := mgr.Execute()
			Expect(err).To(HaveOccurred())
			ensureNoWriteToStateFile()
		})
	})

	Describe("Upgrade", func() {
		Context("When determining whether an upgrade is required exits with an error", func() {
			BeforeEach(func() {
				mgr = createManager(managerArgs{
					NodeCount: 3,
				})

				fakeUpgrader.NeedsUpgradeReturns(false, errors.New("Error determining whether upgrade is required"))
			})

			It("forwards the error", func() {
				err := mgr.Execute()
				Expect(err).To(HaveOccurred())
			})
		})

		Context("When upgrade is required", func() {
			Context("And performing the upgrade exits with an error", func() {

				BeforeEach(func() {
					mgr = createManager(managerArgs{
						NodeCount: 3,
					})

					fakeUpgrader.NeedsUpgradeReturns(true, nil)
					fakeUpgrader.UpgradeReturns(errors.New("Error while performing upgrade"))
				})

				It("forwards the error", func() {
					err := mgr.Execute()
					Expect(err).To(HaveOccurred())
				})
			})
		})
	})

	Describe("SeedDatabases", func() {
		BeforeEach(func() {
			mgr = createManager(managerArgs{
				NodeCount: 1,
			})

			fakeDBHelper.IsDatabaseReachableReturns(true)
		})

		Context("when database seeding fails", func() {
			var expectedErr error
			BeforeEach(func() {
				expectedErr = errors.New("seeding databases failed")
				fakeDBHelper.SeedReturns(expectedErr)
			})

			It("forwards the error", func() {
				err := mgr.Execute()
				Expect(err).To(Equal(expectedErr))
			})
		})
	})

	Context("When starting in single-node deployment", func() {

		BeforeEach(func() {
			mgr = createManager(managerArgs{
				NodeCount: 1,
			})
		})

		Context("And it's an initial deploy", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(false)
			})

			It("bootstraps, seeds databases and writes '"+node_starter.SingleNode+"' to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(node_starter.SingleNode)
				ensureSeedDatabases()
			})

			It("sets the read only user", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				Expect(fakeDBHelper.CreateReadOnlyUserCallCount()).To(Equal(1))
			})
		})

		Context("And it's a redeploy", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(node_starter.SingleNode, nil)
			})

			It("bootstraps, seeds databases and writes '"+node_starter.SingleNode+"' to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(node_starter.SingleNode)
				ensureSeedDatabases()
			})

			It("sets the read only user", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				Expect(fakeDBHelper.CreateReadOnlyUserCallCount()).To(Equal(1))
			})
		})
	})

	Context("When starting in multi-node deployment", func() {

		Context("And it's an initial deploy, so there's no state file", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(false)
			})

			Context("And the IP of the current node is the first in the cluster", func() {

				BeforeEach(func() {
					mgr = createManager(managerArgs{
						NodeCount: 3,
					})

					fakeClusterHealthChecker.HealthyClusterReturns(false)
				})

				It("bootstraps, seeds databases and writes "+node_starter.Clustered+" to file", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureBootstrapWithStateFileContents(node_starter.Clustered)
					ensureSeedDatabases()
				})

				It("sets the read only user", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					Expect(fakeDBHelper.CreateReadOnlyUserCallCount()).To(Equal(1))
				})
			})

			Context("And the IP of the current node is not the first in the cluster", func() {

				BeforeEach(func() {
					mgr = createManager(managerArgs{
						NodeIndex: 1,
						NodeCount: 3,
					})

					fakeClusterHealthChecker.HealthyClusterReturns(false)
				})

				It("joins cluster, seeds databases, and writes '"+node_starter.Clustered+"' to file", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureJoin()
					ensureSeedDatabases()
				})
			})
		})

		Context("When state file is present", func() {
			BeforeEach(func() {
				mgr = createManager(managerArgs{
					NodeCount: 3,
				})

				fakeOs.FileExistsReturns(true)
			})

			Context("And contains extra whitespace characters as well as a valid state", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(fmt.Sprintf("\n\n     %s \n", node_starter.Clustered), nil)
				})

				It("joins the cluster and seeds the databases", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureJoin()
					ensureSeedDatabases()
				})
			})

			Context("And reads '"+node_starter.Clustered+"'", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(node_starter.Clustered, nil)
				})

				It("joins the cluster and seeds the databases", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureJoin()
					ensureSeedDatabases()
				})
			})

			Context("And reads '"+node_starter.NeedsBootstrap+"'", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(node_starter.NeedsBootstrap, nil)
				})

				It("joins cluster, seeds databases, and writes '"+node_starter.Clustered+"' to file", func() {
					err := mgr.Execute()
					Expect(err).NotTo(HaveOccurred())
					ensureBootstrapWithStateFileContents(node_starter.Clustered)
					ensureSeedDatabases()
				})

				Context("And one or more other nodes is reachable", func() {
					BeforeEach(func() {
						fakeClusterHealthChecker.HealthyClusterReturns(true)

						mgr = createManager(managerArgs{
							NodeCount: 3,
						})
					})

					It("joins the cluster and seeds databases", func() {
						err := mgr.Execute()
						Expect(err).ToNot(HaveOccurred())
						ensureJoin()
						ensureSeedDatabases()
					})
				})

				Context("And the IP of the current node is not the first in the cluster", func() {
					BeforeEach(func() {
						mgr = createManager(managerArgs{
							NodeIndex: 1,
							NodeCount: 3,
						})
					})

					It("bootstraps, seeds databases, and writes '"+node_starter.Clustered+"' to file", func() {
						err := mgr.Execute()
						Expect(err).NotTo(HaveOccurred())
						ensureBootstrapWithStateFileContents(node_starter.Clustered)
						ensureSeedDatabases()
					})
				})
			})

			Context("And contains an invalid state", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns("INVALID_STATE", nil)
				})

				It("Forwards the error", func() {
					actualErr := mgr.Execute()
					Expect(actualErr).To(HaveOccurred())
				})

				It("does not join the cluster or seed the databases", func() {
					mgr.Execute()
					Expect(fakeDBHelper.StartMysqldInModeCallCount()).To(Equal(0))
					Expect(fakeDBHelper.SeedCallCount()).To(BeZero())
					ensureNoWriteToStateFile()
				})
			})

			Context("But is unreadable", func() {
				var err error
				BeforeEach(func() {
					err = errors.New("some error")
					fakeOs.ReadFileReturns("", err)
				})

				It("Forwards the error", func() {
					actualErr := mgr.Execute()
					Expect(actualErr).To(HaveOccurred())
					Expect(actualErr).To(Equal(err))
				})

				It("does not join the cluster or seed the databases", func() {
					mgr.Execute()
					Expect(fakeDBHelper.StartMysqldInModeCallCount()).To(Equal(0))
					Expect(fakeDBHelper.SeedCallCount()).To(BeZero())
					ensureNoWriteToStateFile()
				})
			})
		})
	})

	Context("When scaling the cluster", func() {
		Context("And scaling down from many nodes to single", func() {
			BeforeEach(func() {
				mgr = createManager(managerArgs{
					NodeCount: 1,
				})

				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(node_starter.Clustered, nil)
			})

			It("seeds databases, bootstraps node 0 and writes '"+node_starter.SingleNode+"' to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(node_starter.SingleNode)
				ensureSeedDatabases()
			})
		})

		Context("And scaling from one to many nodes", func() {
			BeforeEach(func() {
				mgr = createManager(managerArgs{
					NodeCount: 3,
				})

				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(node_starter.SingleNode, nil)
			})

			It("seeds databases, bootstraps node 0 and writes '"+node_starter.Clustered+"' to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(node_starter.Clustered)
				ensureSeedDatabases()
			})

			It("sets the read only user", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				Expect(fakeDBHelper.CreateReadOnlyUserCallCount()).To(Equal(1))
			})
		})
	})
})
