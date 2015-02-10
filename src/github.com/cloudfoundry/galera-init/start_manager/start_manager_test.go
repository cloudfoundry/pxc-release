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
	dbSeedScriptPath := "/dbSeedScriptPath"
	maxDatabaseSeedTries := 2

	seededDatabases := func() bool {
		callCount := fakeOs.RunCommandCallCount()

		callExists := false

		for i := 0; i < callCount; i++ {
			executable, args := fakeOs.RunCommandArgsForCall(i)

			if executable == "bash" && len(args) > 0 && args[0] == dbSeedScriptPath {
				callExists = true
				break
			}
		}
		return callExists
	}

	ensureSeedDatabases := func() {
		callExists := seededDatabases()
		Expect(callExists).To(BeTrue())
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

	stubPgrepCheck := func(fakeOs *os_fakes.FakeOsHelper) {
		fakeOs.RunCommandWithTimeoutStub = func(_ int, _ string, _ string, args ...string) error {
			if args[0] == "pgrep" {
				return errors.New("did not find the daemon")
			}
			return nil
		}
	}

	ensureBootstrapWithStateFileContents := func(contents string) {
		Expect(fakeDBHelper.StartMysqldInModeCallCount()).To(Equal(1))
		Expect(fakeDBHelper.StartMysqldInModeArgsForCall(0)).To(Equal(BOOTSTRAP_COMMAND))
		ensureStateFileContentIs(contents)
	}

	ensureJoin := func() {
		Expect(fakeDBHelper.StartMysqldInModeCallCount()).To(Equal(1))
		Expect(fakeDBHelper.StartMysqldInModeArgsForCall(0)).To(Equal(JOIN_COMMAND))
		ensureStateFileContentIs(CLUSTERED)
	}

	ensureStop := func() {
		Expect(fakeDBHelper.StopStandaloneMysqlCallCount()).To(Equal(1))
	}

	createManager := func(jobIndex int, numberOfNodes int) *StartManager {
		return New(
			fakeOs,
			fakeDBHelper,
			fakeUpgrader,
			stateFileLocation,
			dbSeedScriptPath,
			jobIndex,
			numberOfNodes,
			testLogger,
			fakeClusterHealthChecker,
			maxDatabaseSeedTries)
	}

	BeforeEach(func() {
		testLogger = lagertest.NewTestLogger("start_manager")
		fakeOs = new(os_fakes.FakeOsHelper)
		fakeClusterHealthChecker = new(health_checker_fakes.FakeClusterHealthChecker)
		fakeUpgrader = new(upgrader_fakes.FakeUpgrader)
		fakeDBHelper = new(db_helper_fakes.FakeDBHelper)
	})

	Context("When there's an error seeding the databases", func() {
		BeforeEach(func() {
			stubPgrepCheck(fakeOs)

			mgr = createManager(0, 1)
		})

		Context("And the total attempts at seeding the database is less than maxDatabaseSeedTries", func() {
			BeforeEach(func() {
				numTries := 0
				fakeOs.RunCommandStub = func(arg1 string, arg2 ...string) (string, error) {
					numTries++
					if numTries < maxDatabaseSeedTries {
						return "", errors.New("seeding databases failed")
					} else {
						return "succeeded", nil
					}
				}
			})

			It("waits and attempts to retry to seed the database", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureSeedDatabases()
			})
		})

		Context("And the total attempts at seeding the database is greater than or equal to maxDatabaseSeedTries", func() {
			var numTries int
			BeforeEach(func() {
				numTries = 0
				fakeOs.RunCommandStub = func(arg1 string, arg2 ...string) (string, error) {
					numTries++
					return "", errors.New("seeding databases failed")
				}
			})

			It("exits and stops mysql (so the deploy fails) and does not write to the state file", func() {
				err := mgr.Execute()
				Expect(err).To(HaveOccurred())
				Expect(numTries).To(Equal(maxDatabaseSeedTries))
				Expect(fakeOs.SleepCallCount()).To(Equal(maxDatabaseSeedTries))
				ensureStop()
				ensureNoWriteToStateFile()
			})
		})
	})

	Context("When starting in single-node deployment", func() {

		BeforeEach(func() {
			stubPgrepCheck(fakeOs)

			mgr = createManager(0, 1)
		})

		Context("And it's an initial deploy", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(false)
			})

			It("bootstraps, seeds databases and writes '"+SINGLE_NODE+"' to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(SINGLE_NODE)
				ensureSeedDatabases()
			})
		})

		Context("And it's a redeploy", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(SINGLE_NODE, nil)
			})

			It("bootstraps, seeds databases and writes '"+SINGLE_NODE+"' to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(SINGLE_NODE)
				ensureSeedDatabases()
			})
		})
	})

	Context("When starting in multi-node deployment on a node > 0", func() {

		BeforeEach(func() {
			stubPgrepCheck(fakeOs)

			mgr = createManager(1, 3)
		})

		It("joins cluster, seeds databases, and writes '"+CLUSTERED+"' to file", func() {
			err := mgr.Execute()
			Expect(err).ToNot(HaveOccurred())
			ensureJoin()
			ensureSeedDatabases()
		})

		Context("When starting mariadb causes an error", func() {
			BeforeEach(func() {
				fakeDBHelper.StartMysqldInModeStub = func(arg0 string) error {
					return errors.New("some error")
				}
			})
			It("forwards the error", func() {
				err := mgr.Execute()
				Expect(err).To(HaveOccurred())
			})
		})
	})

	Context("When starting in multi-node deployment on node 0", func() {

		BeforeEach(func() {
			stubPgrepCheck(fakeOs)
			fakeClusterHealthChecker.HealthyClusterReturns(false)

			mgr = createManager(0, 3)
		})

		Context("And it's an initial deploy", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(false)
			})

			It("bootstraps, seeds databases and writes "+CLUSTERED+" to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(CLUSTERED)
				ensureSeedDatabases()
			})

			Context("When starting mariadb causes an error", func() {
				BeforeEach(func() {
					fakeDBHelper.StartMysqldInModeStub = func(arg0 string) error {
						return errors.New("some error")
					}
				})

				It("forwards the error", func() {
					err := mgr.Execute()
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context("When state file is present", func() {
			Context("And contains extra whitespace characters as well as '"+CLUSTERED+"'", func() {
				BeforeEach(func() {
					fakeOs.FileExistsReturns(true)
					fakeOs.ReadFileReturns(fmt.Sprintf("\n\n     %s \n", CLUSTERED), nil)
				})

				It("joins the cluster and seeds the databases", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureJoin()
					ensureSeedDatabases()
				})
			})
			Context("And reads '"+CLUSTERED+"'", func() {
				BeforeEach(func() {
					fakeOs.FileExistsReturns(true)
					fakeOs.ReadFileReturns(CLUSTERED, nil)
				})

				It("joins the cluster and seeds the databases", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureJoin()
					ensureSeedDatabases()
				})

				Context("And starting mariadb causes an error", func() {
					BeforeEach(func() {
						fakeDBHelper.StartMysqldInModeStub = func(arg0 string) error {
							return errors.New("some error")
						}
					})
					It("forwards the error", func() {
						err := mgr.Execute()
						Expect(err).To(HaveOccurred())
					})
				})

			})

			Context("And reads '"+NEEDS_BOOTSTRAP+"'", func() {
				BeforeEach(func() {
					fakeOs.FileExistsReturns(true)
					fakeOs.ReadFileReturns(NEEDS_BOOTSTRAP, nil)
				})

				Context("And jobIndex == 0", func() {
					BeforeEach(func() {
						stubPgrepCheck(fakeOs)

						mgr = createManager(0, 3)
					})

					It("joins cluster, seeds databases, and writes '"+CLUSTERED+"' to file", func() {
						err := mgr.Execute()
						Expect(err).NotTo(HaveOccurred())
						ensureBootstrapWithStateFileContents(CLUSTERED)
						ensureSeedDatabases()
					})

					Context("And one or more other nodes is reachable", func() {
						BeforeEach(func() {
							fakeClusterHealthChecker.HealthyClusterReturns(true)

							mgr = createManager(0, 3)
						})

						It("joins the cluster and seeds databases", func() {
							err := mgr.Execute()
							Expect(err).ToNot(HaveOccurred())
							ensureJoin()
							ensureSeedDatabases()
						})
					})

					Context("And starting mariadb causes an error", func() {
						BeforeEach(func() {
							fakeDBHelper.StartMysqldInModeStub = func(arg0 string) error {
								return errors.New("some error")
							}
						})
						It("forwards the error", func() {
							err := mgr.Execute()
							Expect(err).To(HaveOccurred())
						})
					})
				})

				Context("And jobIndex > 0", func() {
					BeforeEach(func() {
						stubPgrepCheck(fakeOs)

						mgr = createManager(1, 3)
					})

					It("joins cluster, seeds databases, and writes '"+CLUSTERED+"' to file", func() {
						err := mgr.Execute()
						Expect(err).NotTo(HaveOccurred())
						ensureBootstrapWithStateFileContents(CLUSTERED)
						ensureSeedDatabases()
					})

					Context("And starting mariadb causes an error", func() {
						BeforeEach(func() {
							fakeDBHelper.StartMysqldInModeStub = func(arg0 string) error {
								return errors.New("some error")
							}
						})
						It("forwards the error", func() {
							err := mgr.Execute()
							Expect(err).To(HaveOccurred())
						})
					})
				})
			})

			Context("And contains an invalid state", func() {
				BeforeEach(func() {
					fakeOs.FileExistsReturns(true)
					fakeOs.ReadFileReturns("INVALID_STATE", nil)
				})

				It("does not join the cluster or seed the databases", func() {
					err := mgr.Execute()
					Expect(err).To(HaveOccurred())
					Expect(fakeDBHelper.StartMysqldInModeCallCount()).To(Equal(0))
					Expect(seededDatabases()).To(BeFalse())
					ensureNoWriteToStateFile()
				})
			})
		})
	})

	Context("When scaling the cluster", func() {
		BeforeEach(func() {
			stubPgrepCheck(fakeOs)
		})

		Context("And scaling down from many nodes to single", func() {
			BeforeEach(func() {
				mgr = createManager(0, 1)

				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(CLUSTERED, nil)
			})

			It("seeds databases, bootstraps node 0 and writes '"+SINGLE_NODE+"' to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(SINGLE_NODE)
				ensureSeedDatabases()
			})
		})

		Context("And scaling from one to many nodes", func() {
			BeforeEach(func() {
				mgr = createManager(0, 3)

				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(SINGLE_NODE, nil)
			})

			It("seeds databases, bootstraps node 0 and writes '"+CLUSTERED+"' to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(CLUSTERED)
				ensureSeedDatabases()
			})
		})
	})

	Context("When determining whether upgrade is required exits with an error", func() {
		BeforeEach(func() {
			mgr = createManager(0, 3)

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
				mgr = createManager(0, 3)

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
