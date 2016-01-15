package mariadb_helper

import (
	"fmt"
	"os/exec"

	"database/sql"

	"github.com/cloudfoundry/mariadb_ctrl/config"
	s "github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/seeder"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pivotal-golang/lager"
)

const (
	StopStandaloneCommand = "stop-stand-alone"
	StopCommand           = "stop"
	StatusCommand         = "status"
)

//go:generate counterfeiter . DBHelper

type DBHelper interface {
	StartMysqldInMode(command string) error
	StartMysqlInJoin() (*exec.Cmd, error)
	StartMysqlInJoinMonitored() (*exec.Cmd, chan error)
	StartMysqlInBootstrap() (*exec.Cmd, error)
	StopMysql() error
	StopStandaloneMysql() error
	Upgrade() (output string, err error)
	IsDatabaseReachable() bool
	IsProcessRunning() bool
	Seed() error
	CreateSuperROUser() error
}

type MariaDBHelper struct {
	osHelper        os_helper.OsHelper
	dbSeeder        s.Seeder
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

var BuildSeeder = func(db *sql.DB, config config.PreseededDatabase, logger lager.Logger) s.Seeder {
	return s.NewSeeder(db, config, logger)
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

func (m MariaDBHelper) IsProcessRunning() bool {
	err := m.runMysqlDaemon(StatusCommand)
	if err == nil {
		//exit 0 means process is running
		return true
	}
	return false
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

func (m MariaDBHelper) StartMysqlInJoinMonitored() (*exec.Cmd, chan error) {
	return nil, nil
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

/*
* func (m ) startMysqlAndWait() (error){
	outChannel := make(chan string, 1)
	errChannel := make(chan error, 1)
	 go func() {
		 outChannel, errChannel -> RunCommand(m.logFileLocation, "/var/vcap/packages/mariadb/bin/mysqld_safe", mysqlArgs...)
	 }
	 select {
	 case <-errChannel:
		 return errors.New("Error!!)
	 case <-outChannel:
		 //write to logfile
		 return nil
	 }
	}
*/

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
		seeder := BuildSeeder(db, dbToCreate, m.logger)

		if err := seeder.CreateDBIfNeeded(); err != nil {
			return err
		}

		userAlreadyExists, err := seeder.IsExistingUser()
		if err != nil {
			return err
		}

		if userAlreadyExists == false {
			if err := seeder.CreateUserForDB(); err != nil {
				return err
			}
		}

		if err := seeder.GrantUserAllPrivileges(); err != nil {
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

func (m MariaDBHelper) CreateSuperROUser() error {
	db, err := OpenDBConnection(m.config)
	if err != nil {
		m.logger.Error("database not reachable", err)
		return err
	}
	defer CloseDBConnection(db)
	blankPreSeededDB := &config.PreseededDatabase{
		DBName:   "",
		User:     "",
		Password: "",
	}

	seeder := BuildSeeder(db, *blankPreSeededDB, m.logger)
	err = seeder.CreateUser(m.config.ROUser, m.config.ROPassword)
	if err != nil {
		m.logger.Error("cannot create super RO user", err)
		return err
	}
	err = seeder.GrantUserSuperROPrivileges(m.config.ROUser)
	if err != nil {
		return err
	}
	return nil
}
