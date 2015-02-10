package mariadb_helper

import (
	"fmt"

	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/pivotal-golang/lager"
)

const (
	STOP_STANDALONE_COMMAND = "stop-stand-alone"
)

type DBHelper interface {
	StartMysqldInMode(command string) error
	StopStandaloneMysql() error
	Upgrade() (output string, err error)
	IsDatabaseReachable() bool
}

type MariaDBHelper struct {
	osHelper                os_helper.OsHelper
	mysqlDaemonPath         string
	mysqlClientPath         string
	logFileLocation         string
	logger                  lager.Logger
	upgradeScriptPath       string
	showDatabasesScriptPath string
	username                string
	password                string
}

func NewMariaDBHelper(
	osHelper os_helper.OsHelper,
	mysqlDaemonPath string,
	mysqlClientPath string,
	logFileLocation string,
	logger lager.Logger,
	upgradeScriptPath string,
	showDatabasesScriptPath string,
	username string,
	password string) *MariaDBHelper {
	return &MariaDBHelper{
		osHelper:                osHelper,
		mysqlDaemonPath:         mysqlDaemonPath,
		mysqlClientPath:         mysqlClientPath,
		logFileLocation:         logFileLocation,
		logger:                  logger,
		upgradeScriptPath:       upgradeScriptPath,
		showDatabasesScriptPath: showDatabasesScriptPath,
		username:                username,
		password:                password,
	}
}

func (m MariaDBHelper) StartMysqldInMode(command string) error {
	m.logger.Info("Starting node with '" + command + "' command.")
	err := m.osHelper.RunCommandWithTimeout(10, m.logFileLocation, "bash", m.mysqlDaemonPath, command)
	if err != nil {
		m.logger.Info(fmt.Sprintf("Error starting node: %s", err.Error()))
	}
	return err
}

func (m MariaDBHelper) StopStandaloneMysql() (err error) {
	m.logger.Info("Stopping standalone node")
	err = m.osHelper.RunCommandWithTimeout(10, m.logFileLocation, "bash", m.mysqlDaemonPath, STOP_STANDALONE_COMMAND)
	if err != nil {
		m.logger.Info(fmt.Sprintf("Error stopping node: %s", err.Error()))
	}
	return err
}

func (m MariaDBHelper) Upgrade() (output string, err error) {
	return m.osHelper.RunCommand(
		"bash",
		m.upgradeScriptPath,
		m.username,
		m.password,
		m.logFileLocation)
}

func (m MariaDBHelper) IsDatabaseReachable() bool {
	m.logger.Info(fmt.Sprintf("Determining if database is reachable"))
	output, err := m.osHelper.RunCommand("bash", m.showDatabasesScriptPath, m.mysqlClientPath, m.username, m.password)
	if err != nil {
		m.logger.Info(fmt.Sprintf("database not reachable: %s", output))
		return false
	} else {
		m.logger.Info(fmt.Sprintf("database is reachable"))
		return true
	}
}
