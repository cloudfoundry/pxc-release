package upgrader

import (
	"errors"
	"regexp"
	"time"

	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/pivotal-golang/lager"
)

type Upgrader interface {
	Upgrade() error
	NeedsUpgrade() (bool, error)
}

type upgrader struct {
	packageVersionFile      string
	lastUpgradedVersionFile string
	osHelper                os_helper.OsHelper
	logger                  lager.Logger
	mariadbHelper           mariadb_helper.DBHelper
}

var (
	DBReachablePollingAttempts = 30
	DBReachablePollingDelay    = 10 * time.Second
)

func NewUpgrader(
	packageVersionFile string,
	lastUpgradedVersionFile string,
	osHelper os_helper.OsHelper,
	logger lager.Logger,
	mariadbHelper mariadb_helper.DBHelper) Upgrader {

	return upgrader{
		packageVersionFile:      packageVersionFile,
		lastUpgradedVersionFile: lastUpgradedVersionFile,
		osHelper:                osHelper,
		logger:                  logger,
		mariadbHelper:           mariadbHelper,
	}
}

func (u upgrader) Upgrade() (err error) {
	err = u.startStandaloneDatabaseSynchronously()
	if err != nil {
		u.logger.Info("Synchronously starting standalone database failed.")
	}

	u.logger.Info("Performing upgrade")
	output, upgrade_err := u.mariadbHelper.Upgrade()

	if upgrade_err != nil {
		acceptableErrorsCompiled, _ := regexp.Compile("already upgraded|Unknown command|WSREP has not yet prepared node")
		if acceptableErrorsCompiled.MatchString(output) {
			u.logger.Info("output string matches acceptable errors - continuing startup.")
		} else {
			u.logger.Info("output string does not match acceptable errors - aborting startup.")
			err = upgrade_err
		}
	} else {
		u.logger.Info("Upgrade applied successfully")
	}

	if err != nil {
		return
	}

	err = u.stopStandaloneDatabaseSynchronously()
	if err != nil {
		u.logger.Info("Synchronously stopping standalone database failed.")
	}
	return
}

func (u upgrader) startStandaloneDatabaseSynchronously() (err error) {
	err = u.mariadbHelper.StartMysqldInMode("stand-alone")
	if err != nil {
		u.logger.Info("There was an error starting mysql in stand-alone mode: " + err.Error())
		return
	}

	for tries := 0; tries < DBReachablePollingAttempts; tries++ {
		if u.mariadbHelper.IsDatabaseReachable() {
			return nil
		}

		u.osHelper.Sleep(DBReachablePollingDelay)
	}

	return errors.New("Database is not reachable after 30 tries.")
}

func (u upgrader) stopStandaloneDatabaseSynchronously() (err error) {
	err = u.mariadbHelper.StopStandaloneMysql()
	if err != nil {
		u.logger.Info("Failed to stop standalone MySQL")
		return
	}

	for tries := 0; tries < DBReachablePollingAttempts; tries++ {
		if !u.mariadbHelper.IsDatabaseReachable() {
			return nil
		}

		u.osHelper.Sleep(DBReachablePollingDelay)
	}

	return errors.New("Database is still reachable after 30 tries.")
}

func (u upgrader) NeedsUpgrade() (bool, error) {
	if !u.osHelper.FileExists(u.lastUpgradedVersionFile) {
		u.logger.Info("Last Upgraded version file: '" + u.lastUpgradedVersionFile + "' does not exist in the data dir. Upgrade required.")
		return true, nil
	}

	if !u.osHelper.FileExists(u.packageVersionFile) {
		u.logger.Info("Cannot determine whether upgrade is required. Error reading package version file: '" + u.packageVersionFile + "'. File does not exist.")
		return false, errors.New("MariaDB package is invalid because it is missing the version file.")
	}

	existing_version, err := u.osHelper.ReadFile(u.lastUpgradedVersionFile)
	if err != nil {
		u.logger.Info("Cannot determine whether upgrade is required. Error reading last upgraded version file: '" + u.lastUpgradedVersionFile + "'.")
		return false, errors.New("Could not read last upgraded version file in the data dir.")
	}

	package_version, err := u.osHelper.ReadFile(u.packageVersionFile)
	if err != nil {
		u.logger.Info("Cannot determine whether upgrade is required. Error reading package version file: '" + u.packageVersionFile + "'.")
		return false, errors.New("MariaDB package is invalid because the version file is not readable.")
	}

	if existing_version != package_version {
		u.logger.Info("Need to upgrade to latest version.")
		return true, nil
	}
	u.logger.Info("Already upgraded to latest version, starting normally.")
	return false, nil
}
