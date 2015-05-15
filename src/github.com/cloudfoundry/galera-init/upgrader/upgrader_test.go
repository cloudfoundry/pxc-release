package upgrader_test

import (
	"errors"

	"github.com/cloudfoundry/mariadb_ctrl/config"
	db_fakes "github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/fakes"
	os_fakes "github.com/cloudfoundry/mariadb_ctrl/os_helper/fakes"
	. "github.com/cloudfoundry/mariadb_ctrl/upgrader"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager/lagertest"
)

var _ = Describe("Upgrader", func() {
	var upgrader Upgrader
	var fakeOs *os_fakes.FakeOsHelper
	var fakeDbHelper *db_fakes.FakeDBHelper
	var testLogger *lagertest.TestLogger

	lastUpgradedVersionFile := "/var/vcap/store/mysql/mysql_upgrade_info"
	packageVersionFile := "/var/vcap/package/mariadb/VERSION"

	BeforeEach(func() {
		fakeOs = new(os_fakes.FakeOsHelper)
		fakeDbHelper = new(db_fakes.FakeDBHelper)
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

		It("starts the node is stand-alone mode, runs the upgrade script, then stops the node", func() {
			expectedPollingCounts := DBReachablePollingAttempts + 1
			err := upgrader.Upgrade()
			Expect(fakeDbHelper.StartMysqldInModeCallCount()).To(Equal(1))
			Expect(fakeDbHelper.IsDatabaseReachableCallCount()).To(Equal(expectedPollingCounts))
			Expect(fakeDbHelper.UpgradeCallCount()).To(Equal(1))
			Expect(fakeDbHelper.StopStandaloneMysqlCallCount()).To(Equal(1))
			Expect(err).ToNot(HaveOccurred())
		})

		Context("when the mysql start command fails", func() {
			It("quits early and returns an error", func() {
				fakeDbHelper.StartMysqldInModeStub = func(mode string) error {
					return errors.New("BOOM")
				}
				err := upgrader.Upgrade()
				Expect(err).To(HaveOccurred())
				Expect(fakeDbHelper.IsDatabaseReachableCallCount()).To(Equal(0))
			})
		})

		Context("when the database server is not available after "+string(DBReachablePollingAttempts)+" attempts to reconnect", func() {
			BeforeEach(func() {
				fakeDbHelper.IsDatabaseReachableStub = func() bool {
					return false
				}
			})

			It("returns an error", func() {
				err := upgrader.Upgrade()
				Expect(fakeDbHelper.IsDatabaseReachableCallCount()).To(Equal(DBReachablePollingAttempts))
				Expect(err).To(HaveOccurred())
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

		Context("when the mysql stop script fails", func() {
			BeforeEach(func() {
				fakeDbHelper.StopStandaloneMysqlStub = func() error {
					return errors.New("exited 1")
				}
			})

			It("considers the upgrade a failure", func() {
				err := upgrader.Upgrade()
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when we issue a stop to the DB and it hasn't stopped after polling "+string(DBReachablePollingAttempts)+" times", func() {
			expectedPollingCounts := DBReachablePollingAttempts + 1
			BeforeEach(func() {
				fakeDbHelper.IsDatabaseReachableStub = func() bool {
					return true
				}
			})

			It("returns an error", func() {
				err := upgrader.Upgrade()
				Expect(fakeDbHelper.IsDatabaseReachableCallCount()).To(Equal(expectedPollingCounts))
				Expect(err).To(HaveOccurred())
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

		Context("when the mariadb package version file does not exist", func() {
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

		Context("when we fail to read the last upgraded version file in the MySQL datadir", func() {
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

		Context("when we fail to read the package version file in the MariaDB package", func() {
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

		Context("when the last upgraded version in the MySQL datadir matches the Mariadb package version", func() {
			BeforeEach(func() {
				fakeOs.FileExistsReturns(true)

				fakeOs.ReadFileStub = func(filename string) (string, error) {
					switch filename {
					case lastUpgradedVersionFile:
						return "same version", nil
					case packageVersionFile:
						return "same version", nil
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

		Context("when the version in the MySQL datadir does not match the Mariadb package version", func() {
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
