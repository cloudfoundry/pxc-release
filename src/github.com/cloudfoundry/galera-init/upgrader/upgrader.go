package upgrader

import (
	"errors"
	. "github.com/cloudfoundry/mariadb_ctrl/logger"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"regexp"
	"time"
)

type Upgrader interface {
	Upgrade() error
	NeedsUpgrade() (bool, error)
}

type UpgraderImpl struct {
	upgradeScriptPath       string
	mysqlDaemonPath         string
	packageVersionFile      string
	lastUpgradedVersionFile string
	osHelper                os_helper.OsHelper
	logger                  Logger
	mariadbHelper           mariadb_helper.DBHelper
}

var (
	DB_REACHABLE_POLLING_ATTEMPTS = 30
	DB_REACHABLE_POLLING_DELAY    = 2 * time.Second
)

func NewImpl(
	upgradeScriptPath string,
	mysqlDaemonPath string,
	packageVersionFile string,
	lastUpgradedVersionFile string,
	osHelper os_helper.OsHelper,
	logger Logger,
	mariadbHelper mariadb_helper.DBHelper) *UpgraderImpl {

	return &UpgraderImpl{
		upgradeScriptPath:       upgradeScriptPath,
		mysqlDaemonPath:         mysqlDaemonPath,
		packageVersionFile:      packageVersionFile,
		lastUpgradedVersionFile: lastUpgradedVersionFile,
		osHelper:                osHelper,
		logger:                  logger,
		mariadbHelper:           mariadbHelper,
	}
}

func (u UpgraderImpl) Upgrade() (err error) {
	err = u.startStandaloneDatabaseSynchronously()
	if err != nil {
		u.logger.Log("Synchronously starting standalone database failed.")
	}

	u.logger.Log("Performing upgrade")
	output, upgrade_err := u.mariadbHelper.Upgrade()

	if upgrade_err != nil {
		acceptableErrorsCompiled, _ := regexp.Compile("already upgraded|Unknown command|WSREP has not yet prepared node")
		if acceptableErrorsCompiled.MatchString(output) {
			u.logger.Log("output string matches acceptable errors - continuing startup.")
		} else {
			u.logger.Log("output string does not match acceptable errors - aborting startup.")
			err = upgrade_err
		}
	} else {
		u.logger.Log("Upgrade applied successfully")
	}

	if err != nil {
		return
	}

	err = u.stopStandaloneDatabaseSynchronously()
	if err != nil {
		u.logger.Log("Synchronously stopping standalone database failed.")
	}
	return
}

func (u UpgraderImpl) startStandaloneDatabaseSynchronously() (err error) {
	err = u.mariadbHelper.StartMysqldInMode("stand-alone")
	if err != nil {
		u.logger.Log("There was an error starting mysql in stand-alone mode: " + err.Error())
		return
	}

	for tries := 0; tries < DB_REACHABLE_POLLING_ATTEMPTS; tries++ {
		if u.mariadbHelper.IsDatabaseReachable() {
			return nil
		}

		u.osHelper.Sleep(DB_REACHABLE_POLLING_DELAY)
	}

	return errors.New("Database is not reachable after 30 tries.")
}

func (u UpgraderImpl) stopStandaloneDatabaseSynchronously() (err error) {
	err = u.mariadbHelper.StopStandaloneMysql()
	if err != nil {
		u.logger.Log("Failed to stop standalone MySQL")
		return
	}

	for tries := 0; tries < DB_REACHABLE_POLLING_ATTEMPTS; tries++ {
		if !u.mariadbHelper.IsDatabaseReachable() {
			return nil
		}

		u.osHelper.Sleep(DB_REACHABLE_POLLING_DELAY)
	}

	return errors.New("Database is still reachable after 30 tries.")
}

func (u UpgraderImpl) NeedsUpgrade() (bool, error) {
	if !u.osHelper.FileExists(u.lastUpgradedVersionFile) {
		u.logger.Log("Last Upgraded version file: '" + u.lastUpgradedVersionFile + "' does not exist in the data dir. Upgrade required.")
		return true, nil
	}

	if !u.osHelper.FileExists(u.packageVersionFile) {
		u.logger.Log("Cannot determine whether upgrade is required. Error reading package version file: '" + u.packageVersionFile + "'. File does not exist.")
		return false, errors.New("MariaDB package is invalid because it is missing the version file.")
	}

	existing_version, err := u.osHelper.ReadFile(u.lastUpgradedVersionFile)
	if err != nil {
		u.logger.Log("Cannot determine whether upgrade is required. Error reading last upgraded version file: '" + u.lastUpgradedVersionFile + "'.")
		return false, errors.New("Could not read last upgraded version file in the data dir.")
	}

	package_version, err := u.osHelper.ReadFile(u.packageVersionFile)
	if err != nil {
		u.logger.Log("Cannot determine whether upgrade is required. Error reading package version file: '" + u.packageVersionFile + "'.")
		return false, errors.New("MariaDB package is invalid because the version file is not readable.")
	}

	if existing_version != package_version {
		u.logger.Log("Need to upgrade to latest version.")
		return true, nil
	} else {
		u.logger.Log("Already upgraded to latest version, starting normally.")
		return false, nil
	}
}
