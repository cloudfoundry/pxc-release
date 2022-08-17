package db_helper

import (
	"database/sql"
	"fmt"
	"os/exec"

	"code.cloudfoundry.org/lager"

	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/os_helper"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . DBHelper
type DBHelper interface {
	StartMysqldInJoin() (*exec.Cmd, error)
	StartMysqldInBootstrap() (*exec.Cmd, error)
	StopMysqld()
	IsDatabaseReachable() bool
	IsProcessRunning() bool
}

type GaleraDBHelper struct {
	osHelper        os_helper.OsHelper
	logFileLocation string
	logger          lager.Logger
	config          *config.DBHelper
}

func NewDBHelper(
	osHelper os_helper.OsHelper,
	config *config.DBHelper,
	logFileLocation string,
	logger lager.Logger) *GaleraDBHelper {
	return &GaleraDBHelper{
		osHelper:        osHelper,
		config:          config,
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

// Overridable methods to allow mocking DB connections in tests
var OpenDBConnection = func(config *config.DBHelper) (*sql.DB, error) {
	db, err := sql.Open("mysql", FormatDSN(*config))
	if err != nil {
		return nil, err
	}

	return db, nil
}
var CloseDBConnection = func(db *sql.DB) error {
	return db.Close()
}

func (m GaleraDBHelper) IsProcessRunning() bool {
	_, err := m.osHelper.RunCommand(
		"mysqladmin",
		"--defaults-file=/var/vcap/jobs/pxc-mysql/config/mylogin.cnf",
		"status")
	return err == nil
}

func (m GaleraDBHelper) StartMysqldInJoin() (*exec.Cmd, error) {
	m.logger.Info("Starting mysqld with 'join'.")
	cmd, err := m.startMysqldAsChildProcess()

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

	db, err := OpenDBConnection(m.config)
	if err != nil {
		m.logger.Info("database not reachable", lager.Data{"err": err.Error()})
		return false
	}
	defer CloseDBConnection(db)

	var (
		unused string
		value  string
	)

	err = db.QueryRow(`SHOW GLOBAL VARIABLES LIKE 'wsrep\_provider'`).Scan(&unused, &value)
	if err != nil {
		if err == sql.ErrNoRows {
			m.logger.Info(fmt.Sprintf("Database is reachable, Galera is off"))
			return true
		}
		m.logger.Debug(fmt.Sprintf("Could not connect to database, received: %v", err))
		return false
	}

	if value == "none" {
		m.logger.Info(fmt.Sprintf("Database is reachable, Galera is off"))
		return true
	}

	err = db.QueryRow(`SHOW GLOBAL STATUS LIKE 'wsrep\_local\_state\_comment'`).Scan(&unused, &value)
	if err != nil {
		m.logger.Debug(fmt.Sprintf("Galera state not Synced, received: %v", err))
		return false
	}

	m.logger.Info(fmt.Sprintf("Galera Database state is %s", value))
	return value == "Synced"
}
