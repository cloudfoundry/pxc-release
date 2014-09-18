package mariadb_helper

import (
	"errors"
	"fmt"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"time"
)

const (
	STOP_COMMAND = "stop"
)

type MariaDBHelper struct {
	osHelper          os_helper.OsHelper
	mysqlDaemonPath   string
	logFileLocation   string
	loggingOn         bool
	upgradeScriptPath string
	username          string
	password          string
}

func NewMariaDBHelper(osHelper os_helper.OsHelper,
	mysqlDaemonPath string,
	logFileLocation string,
	loggingOn bool,
	upgradeScriptPath string,
	username string,
	password string) *MariaDBHelper {
	return &MariaDBHelper{
		osHelper:          osHelper,
		mysqlDaemonPath:   mysqlDaemonPath,
		logFileLocation:   logFileLocation,
		loggingOn:         loggingOn,
		upgradeScriptPath: upgradeScriptPath,
		username:          username,
		password:          password,
	}
}

func (m *MariaDBHelper) StartMysqldInMode(command string) error {
	m.log("Starting node with '" + command + "' command.\n")
	err := m.osHelper.RunCommandWithTimeout(10, m.logFileLocation, "bash", "pgrep", "-f", m.mysqlDaemonPath)
	if err == nil {
		// Nil error corresponds to a zero exit code - when the process does exist
		m.log("MySQL daemon already started, this is where we should return without starting another\n")
		return errors.New("MariaDB daemon " + m.mysqlDaemonPath + " is already running")
	} else {
		m.log("MySQL daemon not already started, continuing to start it\n")
	}

	err = m.osHelper.RunCommandWithTimeout(10, m.logFileLocation, "bash", m.mysqlDaemonPath, command)
	if err != nil {
		m.log(fmt.Sprintf("Error starting node: %s\n", err.Error()))
	}
	return err
}

func (m *MariaDBHelper) StopMysqld() error {
	m.log("STOPPING NODE.\n")
	err := m.osHelper.RunCommandWithTimeout(10, m.logFileLocation, "bash", m.mysqlDaemonPath, STOP_COMMAND)
	if err != nil {
		m.log("Error stopping node: " + err.Error())
	}
	return err
}

func (m *MariaDBHelper) Upgrade() (output string, err error) {
	return m.osHelper.RunCommand(
		"bash",
		m.upgradeScriptPath,
		m.username,
		m.password,
		m.logFileLocation)
}

func (m *MariaDBHelper) log(info string) {
	if m.loggingOn {
		fmt.Printf("%v ----- %v", time.Now().Local(), info)
	}
}
