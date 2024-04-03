package db_helper

import (
	"database/sql"
	"errors"
	"fmt"
	"os/exec"
	"regexp"

	"code.cloudfoundry.org/lager/v3"

	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/os_helper"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . DBHelper
type DBHelper interface {
	StartMysqldInJoin() (*exec.Cmd, error)
	StartMysqldInBootstrap() (*exec.Cmd, error)
	StopMysqld()
	IsDatabaseReachable() bool
	IsDatabaseSynced() bool
	IsProcessRunning() bool
	SetVariable(name string, value any) error
}

type GaleraDBHelper struct {
	osHelper        os_helper.OsHelper
	logFileLocation string
	logger          lager.Logger
	db              *sql.DB
}

func NewDBHelper(
	osHelper os_helper.OsHelper,
	db *sql.DB,
	logFileLocation string,
	logger lager.Logger) *GaleraDBHelper {
	return &GaleraDBHelper{
		osHelper:        osHelper,
		db:              db,
		logFileLocation: logFileLocation,
		logger:          logger,
	}
}

func FormatDSN(config config.DBHelper) string {
	skipBinLog := ""
	if config.SkipBinlog {
		skipBinLog = "?sql_log_bin=off"
	}
	return fmt.Sprintf("%s:%s@unix(%s)/%s", config.User, config.Password, config.Socket, skipBinLog)
}

func (m GaleraDBHelper) IsProcessRunning() bool {
	_, err := m.osHelper.RunCommand(
		"mysqladmin",
		"--defaults-file=/var/vcap/jobs/pxc-mysql/config/mylogin.cnf",
		"status")
	return err == nil
}

var seqNoRegex = regexp.MustCompile(`WSREP. Recovered position: (.*:-?\d+)`)

func (m GaleraDBHelper) wsrepRecoverStartPosition() (uuidSeqno string, found bool) {
	output, err := m.osHelper.RunCommand("mysqld",
		"--defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf",
		"--wsrep-recover",
		"--disable-log-error",
	)
	if err != nil {
		return "", false
	}

	matches := seqNoRegex.FindStringSubmatch(output)

	// Two matches here mean the regex found the pattern.
	// The first match is the entire string: "WSREP: Recovered position: $UUID:$SEQNO"
	// The second match will be the capturing group from the above regex: "$UUID:$SEQNO"
	if len(matches) != 2 {
		return "", false
	}

	uuidSeqno = matches[1]

	// This "zero" starting position is the default, if no state could be recovered from transaction logs
	// When the null starting position is found, this means incremental state transfer will not proceed, so no sense
	// setting the wsrep start position.
	const wsrepStartPositionZero = "00000000-0000-0000-0000-000000000000:-1"

	if uuidSeqno == wsrepStartPositionZero {
		return "", false
	}

	return uuidSeqno, true
}

func (m GaleraDBHelper) StartMysqldInJoin() (*exec.Cmd, error) {
	m.logger.Info("Starting mysqld with 'join'.")

	var mysqldArgs []string
	if uuidSeqno, found := m.wsrepRecoverStartPosition(); found {
		mysqldArgs = append(mysqldArgs, "--wsrep-start-position="+uuidSeqno)
	}

	cmd, err := m.startMysqldAsChildProcess(mysqldArgs...)
	if err != nil {
		m.logger.Info(fmt.Sprintf("Error starting mysqld: %s", err.Error()))
		return nil, err
	}
	return cmd, nil
}

func (m GaleraDBHelper) StartMysqldInBootstrap() (*exec.Cmd, error) {
	m.logger.Info("Starting mysql with 'bootstrap'.")
	cmd, err := m.startMysqldAsChildProcess("--wsrep-new-cluster")

	if err != nil {
		m.logger.Info(fmt.Sprintf("Error starting node with 'bootstrap': %s", err.Error()))
		return nil, err
	}
	return cmd, nil
}

func (m GaleraDBHelper) StopMysqld() {
	m.logger.Info("Stopping node")
	_, err := m.osHelper.RunCommand(
		"mysqladmin",
		"--defaults-file=/var/vcap/jobs/pxc-mysql/config/mylogin.cnf",
		"shutdown")
	if err != nil {
		m.logger.Fatal("Error stopping mysqld", err)
	}
}

func (m GaleraDBHelper) startMysqldAsChildProcess(mysqlArgs ...string) (*exec.Cmd, error) {
	args := append(
		[]string{
			"--defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf",
			"--defaults-group-suffix=_plugin",
		},
		mysqlArgs...,
	)
	return m.osHelper.StartCommand(m.logFileLocation, "mysqld", args...)
}

func (m GaleraDBHelper) IsDatabaseReachable() bool {
	m.logger.Info(fmt.Sprintf("Determining if database is reachable"))

	if err := m.db.Ping(); err != nil {
		m.logger.Error("database ping failed", err)
		return false
	}

	m.logger.Info("database ping succeeded. database is reachable.")

	return true
}

// TODO: This should be tested
func (m GaleraDBHelper) IsDatabaseSynced() bool {
	m.logger.Info(fmt.Sprintf("Determining if database is a galera synced node"))
	var wsrepProvider string
	if err := m.db.QueryRow(`SELECT @@global.wsrep_provider`).Scan(&wsrepProvider); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			m.logger.Info("Database is reachable, Galera is off")
			return true
		}
		m.logger.Debug(fmt.Sprintf("Could not connect to database, received: %v", err))
		return false
	}

	if wsrepProvider == "none" {
		m.logger.Info("Database is reachable, Galera is off")
		return true
	}

	var wsrepLocalNodeState string
	if err := m.db.QueryRow(`SELECT VARIABLE_VALUE FROM performance_schema.global_status WHERE VARIABLE_NAME = 'wsrep_local_state_comment'`).Scan(&wsrepLocalNodeState); err != nil {
		m.logger.Debug(fmt.Sprintf("Galera state not Synced, received: %v", err))
		return false
	}

	m.logger.Info(fmt.Sprintf("Galera Database state is %s", wsrepLocalNodeState))
	return wsrepLocalNodeState == "Synced"
}

func (m GaleraDBHelper) SetVariable(name string, value any) error {
	if _, err := m.db.Exec(`SET GLOBAL `+name+` = ?`, value); err != nil {
		return fmt.Errorf("failed to set global variable %q=%v: %s", name, value, err)
	}

	return nil
}
