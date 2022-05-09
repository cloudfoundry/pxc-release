package upgrader_test

import (
	"errors"
	"os/exec"

	"code.cloudfoundry.org/lager/lagertest"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/db_helper/db_helperfakes"
	"github.com/cloudfoundry/galera-init/os_helper/os_helperfakes"
	. "github.com/cloudfoundry/galera-init/upgrader"
)

var _ = Describe("Upgrader", func() {
	var upgrader Upgrader
	var fakeOs *os_helperfakes.FakeOsHelper
	var fakeDbHelper *db_helperfakes.FakeDBHelper
	var testLogger *lagertest.TestLogger

	lastUpgradedVersionFile := "/var/vcap/store/pxc-mysql/mysql_upgrade_info"
	packageVersionFile := "/var/vcap/package/db_package/VERSION"

	BeforeEach(func() {
		fakeOs = new(os_helperfakes.FakeOsHelper)
		fakeDbHelper = new(db_helperfakes.FakeDBHelper)
		testLogger = lagertest.NewTestLogger("upgrader")

		upgrader = NewUpgrader(
			fakeOs,
			config.Upgrader{
				PackageVersionFile:      packageVersionFile,
				LastUpgradedVersionFile: lastUpgradedVersionFile,
			},
			testLogger,
			fakeDbHelper,
		)

		fakeOs.WaitForCommandStub = func(cmd *exec.Cmd) chan error {
			mysqldExitChan := make(chan error, 1)
			mysqldExitChan <- nil
			return mysqldExitChan
		}
	})

	Describe("Upgrade", func() {
		BeforeEach(func() {
			numTries := 0
			fakeDbHelper.IsDatabaseReachableStub = func() bool {
				numTries += 1
				if numTries == DBReachablePollingAttempts {
					return true
				}
				return false
			}
		})

		It("starts mysqld for upgrade, runs the upgrade script, then stops the node", func() {
			expectedPollingCounts := DBReachablePollingAttempts
			err := upgrader.Upgrade()
			Expect(fakeDbHelper.StartMysqldForUpgradeCallCount()).To(Equal(1))
			Expect(fakeDbHelper.IsDatabaseReachableCallCount()).To(Equal(expectedPollingCounts))
			Expect(fakeDbHelper.UpgradeCallCount()).To(Equal(1))
			Expect(fakeDbHelper.StopMysqldCallCount()).To(Equal(1))
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when starting mysqld fails", func() {
			BeforeEach(func() {
				fakeDbHelper.StartMysqldForUpgradeReturns(nil, errors.New(`mysqld not found on path error`))
			})

			It("returns an error", func() {
				err := upgrader.Upgrade()
				Expect(err).To(MatchError(`mysqld not found on path error`))
			})
		})

		Context("when mysqld fails to start in a timely manner", func() {
			BeforeEach(func() {
				fakeDbHelper.IsDatabaseReachableReturns(false)
			})

			It("returns an error", func() {
				err := upgrader.Upgrade()
				Expect(err).To(MatchError(`Database is not reachable after 30 tries.`))
			})
		})

		Context("when the upgrade script returns an acceptable error", func() {
			BeforeEach(func() {
				fakeDbHelper.UpgradeStub = func() (string, error) {
					return "already upgraded", errors.New("exited 1")
				}
			})

			It("considers the upgrade a success", func() {
				err := upgrader.Upgrade()
				Expect(err).ToNot(HaveOccurred())
			})

		})

		Context("when the upgrade script returns an unacceptable error", func() {
			BeforeEach(func() {
				fakeDbHelper.UpgradeStub = func() (string, error) {
					return "unacceptable error", errors.New("exited 1")
				}
			})

			It("considers the upgrade a failure", func() {
				err := upgrader.Upgrade()
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when mysqld fails on shutdown", func() {
			BeforeEach(func() {
				fakeOs.WaitForCommandStub = func(cmd *exec.Cmd) chan error {
					mysqlErrorCh := make(chan error, 1)
					mysqlErrorCh <- errors.New(`mysqld failed`)
					return mysqlErrorCh
				}
			})

			It("returns an error", func() {
				err := upgrader.Upgrade()
				Expect(err).To(MatchError(`mysqld failed during upgrade: mysqld failed`))
			})
		})
	})

	Describe("NeedsUpgrade", func() {
		Context("when the last upgraded version file in the MySQL datadir does not exist", func() {
			It("requires upgrade", func() {
				fakeOs.FileExistsStub = func(filename string) bool {
					switch filename {
					case lastUpgradedVersionFile:
						return false
					}
					return false
				}

				needsUpgrade, err := upgrader.NeedsUpgrade()
				Expect(err).ToNot(HaveOccurred())
				Expect(needsUpgrade).To(BeTrue())
			})
		})

		Context("when the package version file does not exist", func() {
			It("returns the error", func() {
				fakeOs.FileExistsStub = func(filename string) bool {
					switch filename {
					case lastUpgradedVersionFile:
						return true
					case packageVersionFile:
						return false
					}
					return false
				}

				_, err := upgrader.NeedsUpgrade()
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when we fail to read the last upgraded version file in the mysqld datadir", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)

				fakeOs.ReadFileStub = func(filename string) (string, error) {
					switch filename {
					case lastUpgradedVersionFile:
						return "", errors.New("could not be read")
					case packageVersionFile:
						return "new version", nil
					}
					return "", errors.New("unhandled case!")
				}
			})

			It("forwards the error", func() {
				_, err := upgrader.NeedsUpgrade()
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when we fail to read the package version file in the DB package", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)

				fakeOs.ReadFileStub = func(filename string) (string, error) {
					switch filename {
					case lastUpgradedVersionFile:
						return "new version", nil
					case packageVersionFile:
						return "", errors.New("could not be read")
					}
					return "", errors.New("unhandled case!")
				}
			})

			It("forwards the error", func() {
				_, err := upgrader.NeedsUpgrade()
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when the last upgraded version in the mysqld datadir matches the DB package version", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)

				fakeOs.ReadFileStub = func(filename string) (string, error) {
					switch filename {
					case lastUpgradedVersionFile:
						return "same version", nil
					case packageVersionFile:
						return "same version\n", nil
					}
					return "", errors.New("unhandled case!")
				}
			})

			It("returns false", func() {
				needsUpgrade, err := upgrader.NeedsUpgrade()
				Expect(err).ToNot(HaveOccurred())
				Expect(needsUpgrade).To(BeFalse())
			})
		})

		Context("when the version in the mysqld datadir does not match the DB package version", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)

				fakeOs.ReadFileStub = func(filename string) (string, error) {
					switch filename {
					case lastUpgradedVersionFile:
						return "old version", nil
					case packageVersionFile:
						return "new version", nil
					}
					return "", errors.New("unhandled case!")
				}
			})
			It("returns true", func() {
				needsUpgrade, err := upgrader.NeedsUpgrade()
				Expect(err).ToNot(HaveOccurred())
				Expect(needsUpgrade).To(BeTrue())
			})
		})
	})
})
