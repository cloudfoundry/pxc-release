package mariadb_helper

import (
	"fmt"
	"os/exec"

	"database/sql"

	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pivotal-golang/lager"
)

const (
	StopStandaloneCommand = "stop-stand-alone"
	StopCommand           = "stop"
)

type DBHelper interface {
	StartMysqldInMode(command string) error
	StartMysqlInJoin() (*exec.Cmd, error)
	StartMysqlInBootstrap() (*exec.Cmd, error)
	StopMysql() error
	StopStandaloneMysql() error
	Upgrade() (output string, err error)
	IsDatabaseReachable() bool
	Seed() error
}

type MariaDBHelper struct {
	osHelper        os_helper.OsHelper
	logFileLocation string
	logger          lager.Logger
	config          config.DBHelper
}

func NewMariaDBHelper(
	osHelper os_helper.OsHelper,
	config config.DBHelper,
	logFileLocation string,
	logger lager.Logger) *MariaDBHelper {
	return &MariaDBHelper{
		osHelper:        osHelper,
		config:          config,
		logFileLocation: logFileLocation,
		logger:          logger,
	}
}

// Overridable methods to allow mocking DB connections in tests
var OpenDBConnection = func(config config.DBHelper) (*sql.DB, error) {
	db, err := sql.Open("mysql", fmt.Sprintf("%s:%s@/", config.User, config.Password))
	if err != nil {
		return nil, err
	}
	return db, nil
}
var CloseDBConnection = func(db *sql.DB) error {
	return db.Close()
}

func (m MariaDBHelper) StartMysqldInMode(command string) error {
	m.logger.Info("Starting mysqld with '" + command + "' command.")
	err := m.runMysqlDaemon(command)
	if err != nil {
		m.logger.Info(fmt.Sprintf("Error starting node: %s", err.Error()))
	}
	return err
}

func (m MariaDBHelper) StartMysqlInJoin() (*exec.Cmd, error) {
	m.logger.Info("Starting mysqld with 'join'.")
	cmd, err := m.startMysqlAsChildProcess()

	if err != nil {
		m.logger.Info(fmt.Sprintf("Error starting mysqld: %s", err.Error()))
		return nil, err
	}
	return cmd, nil
}

func (m MariaDBHelper) StartMysqlInBootstrap() (*exec.Cmd, error) {
	m.logger.Info("Starting mysql with 'bootstrap'.")
	cmd, err := m.startMysqlAsChildProcess("--wsrep-new-cluster")

	if err != nil {
		m.logger.Info(fmt.Sprintf("Error starting node with 'bootstrap': %s", err.Error()))
		return nil, err
	}
	return cmd, nil
}

func (m MariaDBHelper) StopMysql() error {
	m.logger.Info("Stopping node")
	err := m.runMysqlDaemon(StopCommand)
	if err != nil {
		m.logger.Info(fmt.Sprintf("Error stopping node: %s", err.Error()))
	}
	return err
}

func (m MariaDBHelper) StopStandaloneMysql() error {
	m.logger.Info("Stopping standalone node")
	err := m.runMysqlDaemon(StopStandaloneCommand)
	if err != nil {
		m.logger.Info(fmt.Sprintf("Error stopping standalone node: %s", err.Error()))
	}
	return err
}

func (m MariaDBHelper) runMysqlDaemon(mode string) error {
	return m.osHelper.RunCommandWithTimeout(
		10,
		m.logFileLocation,
		"bash",
		m.config.DaemonPath,
		mode)
}

func (m MariaDBHelper) startMysqlAsChildProcess(mysqlArgs ...string) (*exec.Cmd, error) {
	return m.osHelper.StartCommand(
		m.logFileLocation,
		"/var/vcap/packages/mariadb/bin/mysqld_safe",
		mysqlArgs...)
}

func (m MariaDBHelper) Upgrade() (output string, err error) {
	return m.osHelper.RunCommand(
		m.config.UpgradePath,
		fmt.Sprintf("-u%s", m.config.User),
		fmt.Sprintf("-p%s", m.config.Password),
	)
}

func (m MariaDBHelper) IsDatabaseReachable() bool {
	m.logger.Info(fmt.Sprintf("Determining if database is reachable"))

	db, err := OpenDBConnection(m.config)
	if err != nil {
		m.logger.Info("database not reachable", lager.Data{"err": err})
		return false
	}
	defer CloseDBConnection(db)

	err = db.Ping()
	if err != nil {
		m.logger.Info("database not reachable", lager.Data{"err": err})
		return false
	}

	m.logger.Info(fmt.Sprintf("database is reachable"))
	return true
}

func (m MariaDBHelper) Seed() error {

	if m.config.PreseededDatabases == nil || len(m.config.PreseededDatabases) == 0 {
		m.logger.Info("No preseeded databases specified, skipping seeding.")
		return nil
	}

	m.logger.Info("Preseeding Databases")

	db, err := OpenDBConnection(m.config)
	if err != nil {
		m.logger.Error("database not reachable", err)
		return err
	}
	defer CloseDBConnection(db)

	for _, dbToCreate := range m.config.PreseededDatabases {
		_, err := db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", dbToCreate.DBName))
		if err != nil {
			m.logger.Error("Error creating preseeded database", err, lager.Data{"dbName": dbToCreate.DBName})
			return err
		}

		rows, err := db.Query(fmt.Sprintf(
			"SELECT User FROM mysql.user WHERE User = '%s'",
			dbToCreate.User))
		if err != nil {
			m.logger.Error("Error getting list of users", err, lager.Data{
				"dbName": dbToCreate.DBName,
			})
			return err
		}
		userAlreadyExists := rows.Next()

		if userAlreadyExists == false {
			_, err = db.Exec(fmt.Sprintf(
				"CREATE USER `%s` IDENTIFIED BY '%s'",
				dbToCreate.User,
				dbToCreate.Password))
			if err != nil {
				m.logger.Error("Error creating user", err, lager.Data{
					"user": dbToCreate.User,
				})
				return err
			}
		}

		_, err = db.Exec(fmt.Sprintf(
			"GRANT ALL ON `%s`.* TO `%s`",
			dbToCreate.DBName,
			dbToCreate.User))
		if err != nil {
			m.logger.Error("Error granting user privileges", err, lager.Data{
				"dbName": dbToCreate.DBName,
				"user":   dbToCreate.User,
			})
			return err
		}

	}

	_, err = db.Exec("FLUSH PRIVILEGES")
	if err != nil {
		m.logger.Error("Error flushing privileges", err)
		return err
	}

	return nil
}
