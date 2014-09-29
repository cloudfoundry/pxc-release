package mariadb_helper

import (
	"fmt"
	. "github.com/cloudfoundry/mariadb_ctrl/logger"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
)

const (
	STOP_COMMAND = "stop"
)

type DBHelper interface {
	StartMysqldInMode(command string) error
	StopMysqld() error
	Upgrade() (output string, err error)
	IsDatabaseReachable() bool
}

type MariaDBHelper struct {
	osHelper                os_helper.OsHelper
	mysqlDaemonPath         string
	mysqlClientPath         string
	logFileLocation         string
	logger                  Logger
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
	logger Logger,
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
	m.logger.Log("Starting node with '" + command + "' command.")
	err := m.osHelper.RunCommandWithTimeout(10, m.logFileLocation, "bash", m.mysqlDaemonPath, command)
	if err != nil {
		m.logger.Log(fmt.Sprintf("Error starting node: %s", err.Error()))
	}
	return err
}

func (m MariaDBHelper) StopMysqld() error {
	m.logger.Log("STOPPING NODE.")
	err := m.osHelper.RunCommandWithTimeout(10, m.logFileLocation, "bash", m.mysqlDaemonPath, STOP_COMMAND)
	if err != nil {
		m.logger.Log("Error stopping node: " + err.Error())
	}
	// TODO: We should wait until the database is actually down before continuing
	// Maybe this could be accomplished by polling the database and returning when poll fails?
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
	output, err := m.osHelper.RunCommand("bash", m.showDatabasesScriptPath, m.mysqlClientPath, m.username, m.password)
	m.logger.Log(fmt.Sprintf("output: %s", output))
	if err != nil {
		m.logger.Log(fmt.Sprintf("error: %s", err))
	}
	return err == nil
}
