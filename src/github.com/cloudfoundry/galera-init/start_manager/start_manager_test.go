package start_manager_test

import (
	"errors"
	"fmt"

	health_checker_fakes "github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker/fakes"
	db_helper_fakes "github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/fakes"
	os_fakes "github.com/cloudfoundry/mariadb_ctrl/os_helper/fakes"
	upgrader_fakes "github.com/cloudfoundry/mariadb_ctrl/upgrader/fakes"
	"github.com/pivotal-golang/lager/lagertest"

	. "github.com/cloudfoundry/mariadb_ctrl/start_manager"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("StartManager", func() {

	var mgr *StartManager

	var testLogger *lagertest.TestLogger
	var fakeOs *os_fakes.FakeOsHelper
	var fakeClusterHealthChecker *health_checker_fakes.FakeClusterHealthChecker
	var fakeUpgrader *upgrader_fakes.FakeUpgrader
	var fakeDBHelper *db_helper_fakes.FakeDBHelper

	stateFileLocation := "/stateFileLocation"
	maxDatabaseSeedTries := 2

	type managerArgs struct {
		AzIndex   int
		JobIndex  int
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
		Expect(fakeDBHelper.StartMysqldInModeCallCount()).To(Equal(1))
		Expect(fakeDBHelper.StartMysqldInModeArgsForCall(0)).To(Equal(BootstrapCommand))
		ensureStateFileContentIs(contents)
	}

	ensureJoin := func() {
		Expect(fakeDBHelper.StartMysqldInModeCallCount()).To(Equal(1))
		Expect(fakeDBHelper.StartMysqldInModeArgsForCall(0)).To(Equal(JoinCommand))
		ensureStateFileContentIs(Clustered)
	}

	ensureStop := func() {
		Expect(fakeDBHelper.StopStandaloneMysqlCallCount()).To(Equal(1))
	}

	createManager := func(args managerArgs) *StartManager {

		//clusterIps does not include the current node's IP, so skip i = 0
		clusterIps := []string{}
		for i := 1; i < args.NodeCount; i++ {
			clusterIps = append(clusterIps, "myIp")
		}

		return New(
			fakeOs,
			Config{
				StateFileLocation:    stateFileLocation,
				AzIndex:              args.AzIndex,
				JobIndex:             args.JobIndex,
				ClusterIps:           clusterIps,
				MaxDatabaseSeedTries: maxDatabaseSeedTries,
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
	})

	Context("When starting mariadb with StartMysqldInMode causes an error", func() {
		BeforeEach(func() {
			mgr = createManager(managerArgs{
				AzIndex:   0,
				JobIndex:  0,
				NodeCount: 3,
			})
			fakeDBHelper.StartMysqldInModeStub = func(arg0 string) error {
				return errors.New("some error")
			}
		})
		It("forwards the error", func() {
			err := mgr.Execute()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("Upgrade", func() {
		Context("When determining whether upgrade is required with NeedsUpgrade exits with an error", func() {
			BeforeEach(func() {
				mgr = createManager(managerArgs{
					AzIndex:   0,
					JobIndex:  0,
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
						AzIndex:   0,
						JobIndex:  0,
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
		Context("When there's an error seeding the databases", func() {
			BeforeEach(func() {
				mgr = createManager(managerArgs{
					AzIndex:   0,
					JobIndex:  0,
					NodeCount: 1,
				})
			})

			Context("And the total attempts at seeding the database is less than maxDatabaseSeedTries", func() {
				BeforeEach(func() {
					numTries := 0

					fakeDBHelper.SeedStub = func() error {
						numTries++
						if numTries < maxDatabaseSeedTries {
							return errors.New("seeding databases failed")
						} else {
							return nil
						}
					}
				})

				It("waits and attempts to retry to seed the database", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					Expect(fakeDBHelper.SeedCallCount()).To(Equal(maxDatabaseSeedTries))
				})
			})

			Context("And the total attempts at seeding the database is greater than or equal to maxDatabaseSeedTries", func() {
				var numTries int
				BeforeEach(func() {
					numTries = 0
					fakeDBHelper.SeedStub = func() error {
						numTries++
						return errors.New("seeding databases failed")
					}
				})

				It("exits and stops mysql (so the deploy fails) and does not write to the state file", func() {
					err := mgr.Execute()
					Expect(err).To(HaveOccurred())
					Expect(numTries).To(Equal(maxDatabaseSeedTries))
					Expect(fakeDBHelper.SeedCallCount()).To(Equal(maxDatabaseSeedTries))
					ensureStop()
					ensureNoWriteToStateFile()
				})
			})
		})
	})

	Context("When starting in single-node deployment", func() {

		BeforeEach(func() {
			mgr = createManager(managerArgs{
				AzIndex:   0,
				JobIndex:  0,
				NodeCount: 1,
			})
		})

		Context("And it's an initial deploy", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(false)
			})

			It("bootstraps, seeds databases and writes '"+SingleNode+"' to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(SingleNode)
				ensureSeedDatabases()
			})
		})

		Context("And it's a redeploy", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(SingleNode, nil)
			})

			It("bootstraps, seeds databases and writes '"+SingleNode+"' to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(SingleNode)
				ensureSeedDatabases()
			})
		})
	})

	Context("When starting in multi-node deployment", func() {

		Context("And it's an initial deploy, so there's no state file", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(false)
			})

			Context("And jobIndex == 0", func() {

				Context("And azIndex == 0", func() {

					BeforeEach(func() {
						mgr = createManager(managerArgs{
							AzIndex:   0,
							JobIndex:  0,
							NodeCount: 3,
						})

						fakeClusterHealthChecker.HealthyClusterReturns(false)
					})

					It("bootstraps, seeds databases and writes "+Clustered+" to file", func() {
						err := mgr.Execute()
						Expect(err).ToNot(HaveOccurred())
						ensureBootstrapWithStateFileContents(Clustered)
						ensureSeedDatabases()
					})
				})

				Context("And azIndex > 0", func() {

					BeforeEach(func() {
						mgr = createManager(managerArgs{
							AzIndex:   1,
							JobIndex:  0,
							NodeCount: 3,
						})

						fakeClusterHealthChecker.HealthyClusterReturns(false)
					})

					It("joins cluster, seeds databases, and writes '"+Clustered+"' to file", func() {
						err := mgr.Execute()
						Expect(err).ToNot(HaveOccurred())
						ensureJoin()
						ensureSeedDatabases()
					})
				})
			})

			Context("And jobIndex > 0", func() {

				BeforeEach(func() {
					mgr = createManager(managerArgs{
						AzIndex:   0,
						JobIndex:  1,
						NodeCount: 3,
					})
				})

				It("joins cluster, seeds databases, and writes '"+Clustered+"' to file", func() {
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
					AzIndex:   0,
					JobIndex:  0,
					NodeCount: 3,
				})

				fakeOs.FileExistsReturns(true)
			})

			Context("And contains extra whitespace characters as well as a valid state", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(fmt.Sprintf("\n\n     %s \n", Clustered), nil)
				})

				It("joins the cluster and seeds the databases", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureJoin()
					ensureSeedDatabases()
				})
			})

			Context("And reads '"+Clustered+"'", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(Clustered, nil)
				})

				It("joins the cluster and seeds the databases", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureJoin()
					ensureSeedDatabases()
				})
			})

			Context("And reads '"+NeedsBootstrap+"'", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns(NeedsBootstrap, nil)
				})

				It("joins cluster, seeds databases, and writes '"+Clustered+"' to file", func() {
					err := mgr.Execute()
					Expect(err).NotTo(HaveOccurred())
					ensureBootstrapWithStateFileContents(Clustered)
					ensureSeedDatabases()
				})

				Context("And one or more other nodes is reachable", func() {
					BeforeEach(func() {
						fakeClusterHealthChecker.HealthyClusterReturns(true)

						mgr = createManager(managerArgs{
							AzIndex:   0,
							JobIndex:  0,
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

				Context("And jobIndex > 0", func() {
					BeforeEach(func() {
						mgr = createManager(managerArgs{
							AzIndex:   0,
							JobIndex:  1,
							NodeCount: 3,
						})
					})

					It("joins cluster, seeds databases, and writes '"+Clustered+"' to file", func() {
						err := mgr.Execute()
						Expect(err).NotTo(HaveOccurred())
						ensureBootstrapWithStateFileContents(Clustered)
						ensureSeedDatabases()
					})
				})
			})

			Context("And contains an invalid state", func() {
				BeforeEach(func() {
					fakeOs.ReadFileReturns("INVALID_STATE", nil)
				})

				It("does not join the cluster or seed the databases", func() {
					err := mgr.Execute()
					Expect(err).To(HaveOccurred())
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
					AzIndex:   0,
					JobIndex:  0,
					NodeCount: 1,
				})

				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(Clustered, nil)
			})

			It("seeds databases, bootstraps node 0 and writes '"+SingleNode+"' to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(SingleNode)
				ensureSeedDatabases()
			})
		})

		Context("And scaling from one to many nodes", func() {
			BeforeEach(func() {
				mgr = createManager(managerArgs{
					AzIndex:   0,
					JobIndex:  0,
					NodeCount: 3,
				})

				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(SingleNode, nil)
			})

			It("seeds databases, bootstraps node 0 and writes '"+Clustered+"' to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(Clustered)
				ensureSeedDatabases()
			})
		})
	})

})
