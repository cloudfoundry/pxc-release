package db_helper

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"os/exec"

	"code.cloudfoundry.org/lager"
	"github.com/go-sql-driver/mysql"
	"github.com/pkg/errors"

	"github.com/cloudfoundry/galera-init/config"
	s "github.com/cloudfoundry/galera-init/db_helper/seeder"
	"github.com/cloudfoundry/galera-init/os_helper"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . DBHelper
type DBHelper interface {
	StartMysqldForUpgrade() (*exec.Cmd, error)
	StartMysqldInJoin() (*exec.Cmd, error)
	StartMysqldInBootstrap() (*exec.Cmd, error)
	StopMysqld()
	Upgrade() (output string, err error)
	IsDatabaseReachable() bool
	IsProcessRunning() bool
	Seed() error
	SeedUsers() error
	RunPostStartSQL() error
}

type GaleraDBHelper struct {
	osHelper        os_helper.OsHelper
	dbSeeder        s.Seeder
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

var BuildSeeder = func(db *sql.DB, config config.PreseededDatabase, logger lager.Logger) s.Seeder {
	return s.NewSeeder(db, config, logger)
}
var BuildUserSeeder = func(db *sql.DB, logger lager.Logger) UserSeeder {
	return NewUserSeeder(db, logger)
}

func FormatDSN(config config.DBHelper) string {
	connectorConfig := mysql.Config{
		User:   config.User,
		Passwd: config.Password,
		Net:    "unix",
		Addr:   config.Socket,
	}
	if config.SkipBinlog {
		connectorConfig.Params = map[string]string{
			"sql_log_bin": "off",
		}
	}

	return connectorConfig.FormatDSN()
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

func (m GaleraDBHelper) StartMysqldForUpgrade() (*exec.Cmd, error) {
	cmd, err := m.osHelper.StartCommand(
		m.logFileLocation,
		"mysqld",
		"--defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf",
		"--wsrep-on=OFF",
		"--wsrep-desync=ON",
		"--wsrep-OSU-method=RSU",
		"--wsrep-provider=none",
		"--skip-networking",
	)

	if err != nil {
		return nil, errors.Wrap(err, "Error starting mysqld in stand-alone")
	}

	return cmd, nil
}

func (m GaleraDBHelper) StartMysqldInJoin() (*exec.Cmd, error) {
	m.logger.Info("Starting mysqld with 'join'.")
	cmd, err := m.startMysqldAsChildProcess("--defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf")

	if err != nil {
		m.logger.Info(fmt.Sprintf("Error starting mysqld: %s", err.Error()))
		return nil, err
	}
	return cmd, nil
}

func (m GaleraDBHelper) StartMysqldInBootstrap() (*exec.Cmd, error) {
	m.logger.Info("Starting mysql with 'bootstrap'.")
	cmd, err := m.startMysqldAsChildProcess("--defaults-file=/var/vcap/jobs/pxc-mysql/config/my.cnf", "--wsrep-new-cluster")

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
	return m.osHelper.StartCommand(
		m.logFileLocation,
		"mysqld",
		mysqlArgs...)
}

func (m GaleraDBHelper) Upgrade() (output string, err error) {
	return m.osHelper.RunCommand(
		m.config.UpgradePath,
		"--defaults-file=/var/vcap/jobs/pxc-mysql/config/mylogin.cnf",
	)
}

func (m GaleraDBHelper) IsDatabaseReachable() bool {
	m.logger.Info(fmt.Sprintf("Determining if database is reachable"))

	db, err := OpenDBConnection(m.config)
	if err != nil {
		m.logger.Info("database not reachable", lager.Data{"err": err})
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

func (m GaleraDBHelper) Seed() error {
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
			if err := seeder.CreateUser(); err != nil {
				return err
			}
		} else {
			if err := seeder.UpdateUser(); err != nil {
				return err
			}
		}

		if err := seeder.GrantUserPrivileges(); err != nil {
			return err
		}
	}

	if err := m.flushPrivileges(db); err != nil {
		return err
	}

	return nil
}

func (m GaleraDBHelper) SeedUsers() error {
	if m.config.SeededUsers == nil || len(m.config.SeededUsers) == 0 {
		m.logger.Info("No seeded users specified, skipping seeding.")
		return nil
	}

	m.logger.Info("Seeding Users")

	db, err := OpenDBConnection(m.config)
	if err != nil {
		m.logger.Error("database not reachable", err)
		return err
	}
	defer CloseDBConnection(db)

	for _, userToCreate := range m.config.SeededUsers {
		seeder := BuildUserSeeder(db, m.logger)

		err = seeder.SeedUser(
			userToCreate.User,
			userToCreate.Password,
			userToCreate.Host,
			userToCreate.Role,
		)
		if err != nil {
			return err
		}

	}

	return nil
}

func (m GaleraDBHelper) flushPrivileges(db *sql.DB) error {
	if _, err := db.Exec("FLUSH PRIVILEGES"); err != nil {
		m.logger.Error("Error flushing privileges", err)
		return err
	}

	return nil
}

func (m GaleraDBHelper) RunPostStartSQL() error {
	m.logger.Info("Running Post Start SQL Queries")

	db, err := OpenDBConnection(m.config)
	if err != nil {
		m.logger.Error("database not reachable", err)
		return err
	}
	defer CloseDBConnection(db)

	for _, file := range m.config.PostStartSQLFiles {
		sqlString, err := ioutil.ReadFile(file)
		if err != nil {
			m.logger.Error("error reading PostStartSQL file", err, lager.Data{
				"filePath": file,
			})
		} else {
			if _, err := db.Exec(string(sqlString)); err != nil {
				return err
			}

		}
	}

	return nil
}
