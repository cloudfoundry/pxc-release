package mariadb_helper

import (
	"fmt"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"time"
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
	loggingOn               bool
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
	loggingOn bool,
	upgradeScriptPath string,
	showDatabasesScriptPath string,
	username string,
	password string) *MariaDBHelper {
	return &MariaDBHelper{
		osHelper:                osHelper,
		mysqlDaemonPath:         mysqlDaemonPath,
		mysqlClientPath:         mysqlClientPath,
		logFileLocation:         logFileLocation,
		loggingOn:               loggingOn,
		upgradeScriptPath:       upgradeScriptPath,
		showDatabasesScriptPath: showDatabasesScriptPath,
		username:                username,
		password:                password,
	}
}

func (m MariaDBHelper) StartMysqldInMode(command string) error {
	m.log("Starting node with '" + command + "' command.\n")
	err := m.osHelper.RunCommandWithTimeout(10, m.logFileLocation, "bash", m.mysqlDaemonPath, command)
	if err != nil {
		m.log(fmt.Sprintf("Error starting node: %s\n", err.Error()))
	}
	return err
}

func (m MariaDBHelper) StopMysqld() error {
	m.log("STOPPING NODE.\n")
	err := m.osHelper.RunCommandWithTimeout(10, m.logFileLocation, "bash", m.mysqlDaemonPath, STOP_COMMAND)
	if err != nil {
		m.log("Error stopping node: " + err.Error())
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

func (m MariaDBHelper) log(info string) {
	if m.loggingOn {
		fmt.Printf("%v ----- %v", time.Now().Local(), info)
	}
}

func (m MariaDBHelper) IsDatabaseReachable() bool {
	output, err := m.osHelper.RunCommand("bash", m.showDatabasesScriptPath, m.mysqlClientPath, m.username, m.password)
	m.log(fmt.Sprintf("output: %s\n", output))
	if err != nil {
		m.log(fmt.Sprintf("error: %s\n", err))
	}
	return err == nil
}
