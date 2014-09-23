package mariadb_start_manager_test

import (
	"errors"
	galera_fakes "github.com/cloudfoundry/mariadb_ctrl/galera_helper/fakes"
	os_fakes "github.com/cloudfoundry/mariadb_ctrl/os_helper/fakes"

	manager "."
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("MariadbStartManager", func() {

	var mgr *manager.MariaDBStartManager
	var fakeOs *os_fakes.FakeOsHelper
	var fakeClusterReachabilityChecker *galera_fakes.FakeClusterReachabilityChecker

	logFileLocation := "/logFileLocation"
	mysqlDaemonPath := "/mysqlDaemonPath"
	mysqlClientPath := "/mysqlClientPath"
	stateFileLocation := "/stateFileLocation"
	username := "fake-username"
	password := "fake-password"
	dbSeedScriptPath := "/dbSeedScriptPath"
	upgradeScriptPath := "/upgradeScriptPath"
	showDatabasesScriptPath := "/showDatabasesScriptPath"
	maxDatabaseSeedTries := 2

	ensureMySQLCommandsRanWithOptions := func(options []string) {
		numMysqlCommandsCalled := 0

		for i := 0; i < fakeOs.RunCommandWithTimeoutCallCount(); i++ {
			option := options[numMysqlCommandsCalled]
			timeout, logFile, executable, args := fakeOs.RunCommandWithTimeoutArgsForCall(i)

			// For this expectation, we only want to consider calls that were made to the mysql daemon
			// (as opposed to other binaries like `pgrep`
			if args[0] != mysqlDaemonPath {
				continue
			}

			numMysqlCommandsCalled += 1
			Expect(timeout).To(Equal(10))
			Expect(logFile).To(Equal(logFileLocation))
			Expect(executable).To(Equal("bash"))
			Expect(args).To(Equal([]string{mysqlDaemonPath, option}))
		}
		Expect(numMysqlCommandsCalled).To(Equal(len(options)))
	}

	ensureUpgrade := func() {
		callCount := fakeOs.RunCommandCallCount()
		var lastCommand string

		for i := 0; i < callCount; i++ {
			executable, args := fakeOs.RunCommandArgsForCall(i)

			if executable == "bash" && len(args) > 0 && args[0] == upgradeScriptPath {
				Expect(args[1]).To(Equal(username))
				Expect(args[2]).To(Equal(password))
				Expect(args[3]).To(Equal(logFileLocation))
				lastCommand = "upgrade"
			}
		}
		Expect(lastCommand).To(Equal("upgrade"))
	}

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

	ensureNoStateFileWritten := func() {
		count := fakeOs.WriteStringToFileCallCount()
		Expect(count).To(Equal(0))
	}

	fakeRestartNOTNeededAfterUpgrade := func() {
		fakeOs.RunCommandStub = func(executable string, args ...string) (string, error) {
			if executable == "bash" && len(args) > 0 && args[0] == upgradeScriptPath {
				return "This installation of MySQL is already upgraded to 10.0.12-MariaDB, use --force if you still need to run mysql_upgrade",
					errors.New("unused error text")
			} else {
				return "", nil
			}
		}
	}

	stubPgrepCheck := func(fakeOs *os_fakes.FakeOsHelper) {
		fakeOs.RunCommandWithTimeoutStub = func(_ int, _ string, _ string, args ...string) error {
			if args[0] == "pgrep" {
				return errors.New("did not find the daemon")
			}
			return nil
		}
	}

	Context("when there's an error seeding the databases", func() {
		BeforeEach(func() {
			fakeOs = new(os_fakes.FakeOsHelper)
			stubPgrepCheck(fakeOs)
			fakeClusterReachabilityChecker = new(galera_fakes.FakeClusterReachabilityChecker)

			mgr = manager.New(
				fakeOs,
				logFileLocation,
				stateFileLocation,
				mysqlDaemonPath,
				mysqlClientPath,
				username,
				password,
				dbSeedScriptPath,
				0, 1, false,
				upgradeScriptPath,
				showDatabasesScriptPath,
				fakeClusterReachabilityChecker,
				maxDatabaseSeedTries)
		})

		Context("and the total attempts at seeding the database is less than maxDatabaseSeedTries", func() {
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

		Context("and the total attempts at seeding the database is more than or equal to maxDatabaseSeedTries", func() {
			var numTries int
			BeforeEach(func() {
				numTries = 0
				fakeOs.RunCommandStub = func(arg1 string, arg2 ...string) (string, error) {
					numTries++
					return "", errors.New("seeding databases failed")
				}
			})

			It("exits and stops mysql (so the deploy fails)", func() {
				err := mgr.Execute()
				Expect(err).To(HaveOccurred())
				Expect(numTries).To(Equal(maxDatabaseSeedTries))
				Expect(fakeOs.SleepCallCount()).To(Equal(maxDatabaseSeedTries))
				ensureMySQLCommandsRanWithOptions([]string{"bootstrap", "stop"})
			})
		})
	})

	Describe("When starting in single-node deployment", func() {

		BeforeEach(func() {
			fakeOs = new(os_fakes.FakeOsHelper)
			stubPgrepCheck(fakeOs)
			fakeClusterReachabilityChecker = new(galera_fakes.FakeClusterReachabilityChecker)

			mgr = manager.New(
				fakeOs,
				logFileLocation,
				stateFileLocation,
				mysqlDaemonPath,
				mysqlClientPath,
				username,
				password,
				dbSeedScriptPath,
				0, 1, false,
				upgradeScriptPath,
				showDatabasesScriptPath,
				fakeClusterReachabilityChecker,
				maxDatabaseSeedTries)
		})

		Context("On initial deploy, when it needs to be restarted after upgrade", func() {
			It("Starts in bootstrap mode, stops, and rebootstraps", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureMySQLCommandsRanWithOptions([]string{"bootstrap", "stop", "bootstrap"})
				ensureStateFileContentIs(manager.SINGLE_NODE)
				ensureUpgrade()
				ensureSeedDatabases()
			})
		})

		Context("When a restart after upgrade is not necessary", func() {
			BeforeEach(func() {
				fakeRestartNOTNeededAfterUpgrade()
			})

			It("Starts in bootstrap mode", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureMySQLCommandsRanWithOptions([]string{"bootstrap"})
				ensureStateFileContentIs(manager.SINGLE_NODE)
				ensureUpgrade()
				ensureSeedDatabases()
			})
		})

		Context("When redeploying, and a restart after upgrade is necessary", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(manager.SINGLE_NODE, nil)
			})
			It("Starts in bootstrap mode", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureMySQLCommandsRanWithOptions([]string{"bootstrap", "stop", "bootstrap"})
				ensureStateFileContentIs(manager.SINGLE_NODE)
				ensureUpgrade()
				ensureSeedDatabases()
			})
		})

	})

	Describe("Execute on node >0", func() {

		BeforeEach(func() {
			fakeOs = new(os_fakes.FakeOsHelper)
			stubPgrepCheck(fakeOs)
			fakeClusterReachabilityChecker = new(galera_fakes.FakeClusterReachabilityChecker)

			mgr = manager.New(
				fakeOs,
				logFileLocation,
				stateFileLocation,
				mysqlDaemonPath,
				mysqlClientPath,
				username,
				password,
				dbSeedScriptPath,
				1, 3, false,
				upgradeScriptPath,
				showDatabasesScriptPath,
				fakeClusterReachabilityChecker,
				maxDatabaseSeedTries)
		})

		Context("When the node joins the cluster", func() {
			It("Should not seed databases", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureNeverSeedDatabases()
			})
		})

		Context("When the node needs to restart after upgrade", func() {
			It("Should start up in join mode, writes "+manager.CLUSTERED+" to a file, runs upgrade, stops mysql", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureMySQLCommandsRanWithOptions([]string{"start", "stop", "start"})
				ensureStateFileContentIs(manager.CLUSTERED)
				ensureUpgrade()
			})

			Context("When starting mariadb causes an error", func() {
				BeforeEach(func() {
					fakeOs.RunCommandWithTimeoutStub = func(arg0 int, arg1 string, arg2 string, arg3 ...string) error {
						return errors.New("some error")
					}
				})

				It("forwards the error", func() {
					err := mgr.Execute()
					Expect(err).To(HaveOccurred())

				})
			})
			Context("When stopping mariadb causes an error", func() {
				BeforeEach(func() {
					fakeOs.RunCommandWithTimeoutStub = func(arg0 int, arg1 string, arg2 string, arg3 ...string) error {
						if arg3[1] == "stop" {
							return errors.New("some errors")
						} else {
							return nil
						}
					}
				})

				It("forwards the error", func() {
					err := mgr.Execute()
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context("When the node does NOT need to restart after upgrade", func() {
			BeforeEach(func() {
				fakeRestartNOTNeededAfterUpgrade()
			})
			It("Should start up in join mode, writes "+manager.CLUSTERED+" to a file, runs upgrade", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureMySQLCommandsRanWithOptions([]string{"start"})
				ensureStateFileContentIs(manager.CLUSTERED)
				ensureUpgrade()
			})
			Context("When starting mariadb causes an error", func() {
				BeforeEach(func() {
					fakeOs.RunCommandWithTimeoutStub = func(arg0 int, arg1 string, arg2 string, arg3 ...string) error {
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

	Describe("Execute on node 0", func() {

		BeforeEach(func() {
			fakeOs = new(os_fakes.FakeOsHelper)
			stubPgrepCheck(fakeOs)
			fakeClusterReachabilityChecker = new(galera_fakes.FakeClusterReachabilityChecker)
			fakeClusterReachabilityChecker.AnyNodesReachableReturns(false)

			mgr = manager.New(
				fakeOs,
				logFileLocation,
				stateFileLocation,
				mysqlDaemonPath,
				mysqlClientPath,
				username,
				password,
				dbSeedScriptPath,
				0, 3, false,
				upgradeScriptPath,
				showDatabasesScriptPath,
				fakeClusterReachabilityChecker,
				maxDatabaseSeedTries)
		})

		Context("When the state file is not present", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(false)
			})

			Context("and upgrade requires restart", func() {
				It("Should bootstrap, upgrade and restart in bootstrap mode", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureMySQLCommandsRanWithOptions([]string{"bootstrap", "stop", "bootstrap"})
					ensureStateFileContentIs(manager.CLUSTERED)
					ensureUpgrade()
					ensureSeedDatabases()
				})
			})

			Context("and upgrade does not require restart", func() {
				BeforeEach(func() {
					fakeRestartNOTNeededAfterUpgrade()
				})

				It("Should bootstrap, upgrade and write "+manager.CLUSTERED+" to file", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureMySQLCommandsRanWithOptions([]string{"bootstrap"})
					ensureUpgrade()
					ensureSeedDatabases()
					ensureStateFileContentIs(manager.CLUSTERED)
				})

			})

			Context("When one or more other nodes is reachable", func() {
				BeforeEach(func() {
					fakeClusterReachabilityChecker = new(galera_fakes.FakeClusterReachabilityChecker)
					fakeClusterReachabilityChecker.AnyNodesReachableReturns(true)

					mgr = manager.New(
						fakeOs,
						logFileLocation,
						stateFileLocation,
						mysqlDaemonPath,
						mysqlClientPath,
						username,
						password,
						dbSeedScriptPath,
						0, 3, false,
						upgradeScriptPath,
						showDatabasesScriptPath,
						fakeClusterReachabilityChecker,
						maxDatabaseSeedTries)
				})

				It("Seeds database, upgrades and restarts", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureMySQLCommandsRanWithOptions([]string{"start", "stop", "start"})
					ensureStateFileContentIs(manager.CLUSTERED)
					ensureUpgrade()
					ensureSeedDatabases()
				})
			})

			Context("When starting mariadb causes an error", func() {
				BeforeEach(func() {
					fakeOs.RunCommandWithTimeoutStub = func(timeout int, logFile string, executable string, args ...string) error {
						return errors.New("some error")
					}
				})

				It("forwards the error", func() {
					err := mgr.Execute()
					Expect(err).To(HaveOccurred())
				})
			})

			Context("when seeding the database fails", func() {
				BeforeEach(func() {
					fakeOs.RunCommandStub = func(executable string, args ...string) (string, error) {
						if args[0] == dbSeedScriptPath {
							return "", errors.New("some error")
						}
						return "success!", nil
					}
				})

				It("does not write a state file", func() {
					err := mgr.Execute()
					Expect(err).To(HaveOccurred())
					ensureNoStateFileWritten()
				})
			})
		})

		Context("When file is present and reads '"+manager.CLUSTERED+"', and upgrade returns err: 'already upgraded'", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(manager.CLUSTERED, nil)
				fakeRestartNOTNeededAfterUpgrade()
			})
			It("Should join, perform upgrade and not restart", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureMySQLCommandsRanWithOptions([]string{"start"})
				ensureSeedDatabases()
				ensureUpgrade()
			})
			Context("When starting mariadb causes an error", func() {
				It("forwards the error", func() {
					fakeOs.RunCommandWithTimeoutStub = func(arg0 int, arg1 string, arg2 string, arg3 ...string) error {
						return errors.New("some error")
					}
					err := mgr.Execute()
					Expect(err).To(HaveOccurred())
				})
			})
		})

		Context("When file is present and reads '"+manager.CLUSTERED+"', and upgrade requires restart", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(manager.CLUSTERED, nil)
			})
			It("Should join, perform upgrade and restart", func() {
				err := mgr.Execute()
				Expect(err).ToNot(HaveOccurred())
				ensureMySQLCommandsRanWithOptions([]string{"start", "stop", "start"})
				ensureStateFileContentIs(manager.CLUSTERED)
				ensureSeedDatabases()
				ensureUpgrade()
			})
			Context("When starting mariadb causes an error", func() {
				It("forwards the error", func() {
					fakeOs.RunCommandWithTimeoutStub = func(arg0 int, arg1 string, arg2 string, arg3 ...string) error {
						return errors.New("some error")
					}
					err := mgr.Execute()
					Expect(err).To(HaveOccurred())
				})
			})
		})
	})

	Describe("When scaling the cluster", func() {
		BeforeEach(func() {
			fakeOs = new(os_fakes.FakeOsHelper)
			stubPgrepCheck(fakeOs)
			fakeClusterReachabilityChecker = new(galera_fakes.FakeClusterReachabilityChecker)
		})

		Context("When scaling down from many nodes to single", func() {
			BeforeEach(func() {
				mgr = manager.New(
					fakeOs,
					logFileLocation,
					stateFileLocation,
					mysqlDaemonPath,
					mysqlClientPath,
					username,
					password,
					dbSeedScriptPath,
					0, 1, false,
					upgradeScriptPath,
					showDatabasesScriptPath,
					fakeClusterReachabilityChecker,
					maxDatabaseSeedTries)

				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(manager.CLUSTERED, nil)
			})
			Context("When restart is needed after upgrade", func() {
				It("Bootstraps node zero and writes '"+manager.SINGLE_NODE+"' to file", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureMySQLCommandsRanWithOptions([]string{"bootstrap", "stop", "bootstrap"})
					ensureStateFileContentIs(manager.SINGLE_NODE)
					ensureSeedDatabases()
					ensureUpgrade()
				})
			})
			Context("When no restart is needed", func() {
				BeforeEach(func() {
					fakeRestartNOTNeededAfterUpgrade()
				})

				It("Bootstraps node zero and writes '"+manager.SINGLE_NODE+"' to file", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureMySQLCommandsRanWithOptions([]string{"bootstrap"})
					ensureStateFileContentIs(manager.SINGLE_NODE)
					ensureSeedDatabases()
					ensureUpgrade()
				})
			})
		})

		Context("Scaling from one to many nodes", func() {
			BeforeEach(func() {
				mgr = manager.New(
					fakeOs,
					logFileLocation,
					stateFileLocation,
					mysqlDaemonPath,
					mysqlClientPath,
					username,
					password,
					dbSeedScriptPath,
					0, 3, false,
					upgradeScriptPath,
					showDatabasesScriptPath,
					fakeClusterReachabilityChecker,
					maxDatabaseSeedTries)

				fakeOs.FileExistsReturns(true)
				fakeOs.ReadFileReturns(manager.SINGLE_NODE, nil)
			})
			Context("When a restart after upgrade is necessary", func() {
				It("bootstraps the first node and writes '"+manager.CLUSTERED+"' to file", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureMySQLCommandsRanWithOptions([]string{"bootstrap", "stop", "bootstrap"})
					ensureStateFileContentIs(manager.CLUSTERED)
					ensureSeedDatabases()
					ensureUpgrade()
				})
			})
			Context("When a restart after upgrade is NOT necessary", func() {
				BeforeEach(func() {
					fakeRestartNOTNeededAfterUpgrade()
				})
				It("bootstraps the first node and writes '"+manager.CLUSTERED+"' to file", func() {
					err := mgr.Execute()
					Expect(err).ToNot(HaveOccurred())
					ensureMySQLCommandsRanWithOptions([]string{"bootstrap"})
					ensureStateFileContentIs(manager.CLUSTERED)
					ensureUpgrade()
					ensureSeedDatabases()
				})
			})
		})
	})
})
