package upgrader

import (
	"errors"
	"fmt"
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
	loggingOn               bool
	mariadbHelper           mariadb_helper.DBHelper
}

func NewImpl(
	upgradeScriptPath string,
	mysqlDaemonPath string,
	packageVersionFile string,
	lastUpgradedVersionFile string,
	osHelper os_helper.OsHelper,
	loggingOn bool,
	mariadbHelper mariadb_helper.DBHelper) *UpgraderImpl {

	return &UpgraderImpl{
		upgradeScriptPath:       upgradeScriptPath,
		mysqlDaemonPath:         mysqlDaemonPath,
		packageVersionFile:      packageVersionFile,
		lastUpgradedVersionFile: lastUpgradedVersionFile,
		osHelper:                osHelper,
		loggingOn:               loggingOn,
		mariadbHelper:           mariadbHelper,
	}
}

func (u UpgraderImpl) Log(info string) {
	if u.loggingOn {
		fmt.Printf("%v ----- %v\n", time.Now().Local(), info)
	}
}

func (u UpgraderImpl) Upgrade() (err error) {
	err = u.mariadbHelper.StartMysqldInMode("stand-alone")
	if err != nil {
		u.Log("There was an error starting mysql in stand-alone mode")
		return
	}

	reachable := false
	for tries := 0; tries < 30; tries++ {
		reachable = u.mariadbHelper.IsDatabaseReachable()
		if reachable {
			break
		}
		u.osHelper.Sleep(2)
	}

	if !reachable {
		err = errors.New("Database is not reachable after 30 tries. Exiting...")
		return
	}

	output, upgrade_err := u.mariadbHelper.Upgrade()

	if upgrade_err != nil {
		acceptableErrorsCompiled, _ := regexp.Compile("already upgraded|Unknown command|WSREP has not yet prepared node")
		if acceptableErrorsCompiled.MatchString(output) {
			u.Log("output string matches acceptable errors - continuing startup\n")
		} else {
			u.Log("output string does not match acceptable errors - aborting startup\n")
			err = upgrade_err
		}
	}

	stop_err := u.mariadbHelper.StopMysqld()
	if stop_err != nil && err == nil {
		err = stop_err
	}
	return
}

func (u UpgraderImpl) NeedsUpgrade() (bool, error) {
	if !u.osHelper.FileExists(u.lastUpgradedVersionFile) {
		u.Log("Version file does not exist in the data dir. Upgrade required")
		return true, nil
	}

	if !u.osHelper.FileExists(u.packageVersionFile) {
		u.Log("Version file does not exist in the MariaDB package. There is something with the package. Cannot determine whether upgrade is required")
		return false, errors.New("MariaDB package is invalid because it is missing its VERSION file")
	}

	existing_version, err := u.osHelper.ReadFile(u.lastUpgradedVersionFile)
	if err != nil {
		u.Log("Error reading last upgraded version file. Cannot determine whether upgrade is required")
		return false, errors.New("Could not read last upgraded version file in the data dir.")
	}

	package_version, err := u.osHelper.ReadFile(u.packageVersionFile)
	if err != nil {
		u.Log("Error reading package version file. Cannot determine whether upgrade is required")
		return false, errors.New("Could not read VERSION file in the MariaDB package.")
	}

	if existing_version != package_version {
		u.Log("Need to upgrade to latest version")
		return true, nil
	} else {
		u.Log("Already upgraded to latest version, starting normally")
		return false, nil
	}
}
