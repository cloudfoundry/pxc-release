package mariadb_start_manager_test

import (
	"errors"
	galera_fakes "github.com/cloudfoundry/mariadb_ctrl/galera_helper/fakes"
	logger_fakes "github.com/cloudfoundry/mariadb_ctrl/logger/fakes"
	db_helper_fakes "github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/fakes"
	os_fakes "github.com/cloudfoundry/mariadb_ctrl/os_helper/fakes"
	upgrader_fakes "github.com/cloudfoundry/mariadb_ctrl/upgrader/fakes"

	. "github.com/cloudfoundry/mariadb_ctrl/mariadb_start_manager"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MariadbStartManager", func() {

	var mgr *MariaDBStartManager

	var fakeLogger *logger_fakes.FakeLogger
	var fakeOs *os_fakes.FakeOsHelper
	var fakeClusterReachabilityChecker *galera_fakes.FakeClusterReachabilityChecker
	var fakeUpgrader *upgrader_fakes.FakeUpgrader
	var fakeDBHelper *db_helper_fakes.FakeDBHelper

	BeforeEach(func() {
		fakeLogger = new(logger_fakes.FakeLogger)
		fakeOs = new(os_fakes.FakeOsHelper)
		fakeClusterReachabilityChecker = new(galera_fakes.FakeClusterReachabilityChecker)
		fakeUpgrader = new(upgrader_fakes.FakeUpgrader)
		fakeDBHelper = new(db_helper_fakes.FakeDBHelper)
	})

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

	ensureNeverSeedDatabases := func() {
		callExists := seededDatabases()
		Expect(callExists).To(BeFalse())
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

	createManager := func(jobIndex int, numberOfNodes int) *MariaDBStartManager {
		return New(
			fakeOs,
			fakeDBHelper,
			fakeUpgrader,
			stateFileLocation,
			dbSeedScriptPath,
			jobIndex,
			numberOfNodes,
			fakeLogger,
			fakeClusterReachabilityChecker,
			maxDatabaseSeedTries)
	}

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

	Describe("When starting in single-node deployment", func() {

		BeforeEach(func() {
			stubPgrepCheck(fakeOs)

			mgr = createManager(0, 1)
		})

		Context("On initial deploy", func() {
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

		Context("When redeploying", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(SINGLE_NODE, nil)
			})

			It("boostraps, seeds databases and writes '"+SINGLE_NODE+"' to file", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureBootstrapWithStateFileContents(SINGLE_NODE)
				ensureSeedDatabases()
			})
		})
	})

	Describe("When starting in multi-node deployment on a node > 0", func() {

		BeforeEach(func() {
			stubPgrepCheck(fakeOs)

			mgr = createManager(1, 3)
		})

		It("joins cluster, does not seed databases, and writes '"+CLUSTERED+"' to file", func() {
			err := mgr.Execute()
			Expect(err).ToNot(HaveOccurred())
			ensureJoin()
			ensureNeverSeedDatabases()
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

	Describe("When starting in multi-node deployment on node 0", func() {

		BeforeEach(func() {
			stubPgrepCheck(fakeOs)
			fakeClusterReachabilityChecker.AnyNodesReachableReturns(false)

			mgr = createManager(0, 3)
		})

		Context("On initial deploy", func() {
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

		Context("When file is present and reads '"+CLUSTERED+"'", func() {
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

			Context("When one or more other nodes is reachable", func() {
				BeforeEach(func() {
					fakeClusterReachabilityChecker.AnyNodesReachableReturns(true)

					mgr = createManager(0, 3)
				})

				It("joins the cluster and seeds databases", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureJoin()
					ensureSeedDatabases()
				})
			})
		})
	})

	Describe("When scaling the cluster", func() {
		BeforeEach(func() {
			stubPgrepCheck(fakeOs)
		})

		Context("When scaling down from many nodes to single", func() {
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

		Context("Scaling from one to many nodes", func() {
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

	Describe("When determining whether upgrade is required exits with an error", func() {
		BeforeEach(func() {
			mgr = createManager(0, 3)

			fakeUpgrader.NeedsUpgradeReturns(false, errors.New("Error determining whether upgrade is required"))
		})

		It("forwards the error", func() {
			err := mgr.Execute()
			Expect(err).To(HaveOccurred())
		})
	})

	Describe("When upgrade is required", func() {

		Context("When performing the upgrade exits with an error", func() {

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
