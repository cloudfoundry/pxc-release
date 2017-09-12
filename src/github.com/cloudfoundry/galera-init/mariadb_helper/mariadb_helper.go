package mariadb_helper

import (
	"fmt"
	"os/exec"

	"database/sql"

	"io/ioutil"

	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	s "github.com/cloudfoundry/mariadb_ctrl/mariadb_helper/seeder"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/go-sql-driver/mysql"
	"time"
)

const (
	StopCommand   = "stop"
	StatusCommand = "status"
)

//go:generate counterfeiter . DBHelper

type DBHelper interface {
	StartMysqldInStandAlone()
	StartMysqldInJoin() (*exec.Cmd, error)
	StartMysqldInBootstrap() (*exec.Cmd, error)
	StopMysqld()
	Upgrade() (output string, err error)
	IsDatabaseReachable() bool
	IsProcessRunning() bool
	Seed() error
	RunPostStartSQL() error
	TestDatabaseCleanup() error
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
	c := mysql.Config{
		User:   config.User,
		Passwd: config.Password,
		Net:    "tcp",
		Addr:   fmt.Sprintf("localhost:%d", config.Port),
	}
	db, err := sql.Open("mysql", c.FormatDSN())
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
	return err == nil
}

func (m MariaDBHelper) StartMysqldInStandAlone() {
	err := m.runMysqlDaemon("stand-alone")
	if err != nil {
		m.logger.Fatal("Error staring mysqld in stand-alone", err)
	}
}

func (m MariaDBHelper) StartMysqldInJoin() (*exec.Cmd, error) {
	m.logger.Info("Starting mysqld with 'join'.")
	cmd, err := m.startMysqldAsChildProcess()

	if err != nil {
		m.logger.Info(fmt.Sprintf("Error starting mysqld: %s", err.Error()))
		return nil, err
	}
	return cmd, nil
}

func (m MariaDBHelper) StartMysqldInBootstrap() (*exec.Cmd, error) {
	m.logger.Info("Starting mysql with 'bootstrap'.")
	cmd, err := m.startMysqldAsChildProcess("--wsrep-new-cluster")

	if err != nil {
		m.logger.Info(fmt.Sprintf("Error starting node with 'bootstrap': %s", err.Error()))
		return nil, err
	}
	return cmd, nil
}

func (m MariaDBHelper) StopMysqld() {
	m.logger.Info("Stopping node")
	err := m.runMysqlDaemon(StopCommand)
	if err != nil {
		m.logger.Fatal("Error stopping mysqld", err)
	}

	for {
		if !m.IsProcessRunning() {
			m.logger.Info("mysqld has been stopped")
			return
		}

		m.logger.Info("mysqld is still running...")
		m.osHelper.Sleep(1 * time.Second)
	}
}

func (m MariaDBHelper) runMysqlDaemon(mode string) error {
	runCommandErr := m.osHelper.RunCommand(
		"bash",
		m.config.DaemonPath,
		mode)

	return runCommandErr
}

func (m MariaDBHelper) startMysqlDaemon(mode string) (*exec.Cmd, error) {
	return m.osHelper.StartCommand(
		m.logFileLocation,
		"bash",
		m.config.DaemonPath,
		mode)
}

func (m MariaDBHelper) startMysqldAsChildProcess(mysqlArgs ...string) (*exec.Cmd, error) {
	return m.osHelper.StartCommand(
		m.logFileLocation,
		"/var/vcap/packages/mariadb/bin/mysqld_safe",
		mysqlArgs...)
}

func (m MariaDBHelper) Upgrade() (output string, err error) {
	return m.osHelper.RunCommandWithOutput(
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

	var (
		unused string
		value  string
	)

	err = db.QueryRow(`SHOW GLOBAL VARIABLES LIKE 'wsrep\_on'`).Scan(&unused, &value)
	if err != nil {
		return false
	}

	if value == "OFF" {
		m.logger.Info(fmt.Sprintf("Database is reachable, Galera is off"))
		return true
	}

	err = db.QueryRow(`SHOW STATUS LIKE 'wsrep\_ready'`).Scan(&unused, &value)
	if err != nil {
		return false
	}

	m.logger.Info(fmt.Sprintf("Database is reachable, Galera is %s", value))
	return value == "ON"
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
			if err := seeder.CreateUser(); err != nil {
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

func (m MariaDBHelper) flushPrivileges(db *sql.DB) error {
	if _, err := db.Exec("FLUSH PRIVILEGES"); err != nil {
		m.logger.Error("Error flushing privileges", err)
		return err
	}

	return nil
}

func (m MariaDBHelper) RunPostStartSQL() error {
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

func (m MariaDBHelper) TestDatabaseCleanup() error {
	db, err := OpenDBConnection(m.config)
	if err != nil {
		panic("")
	}
	defer CloseDBConnection(db)

	err = m.deletePermissionsToCreateTestDatabases(db)
	if err != nil {
		return err
	}

	err = m.flushPrivileges(db)
	if err != nil {
		return err
	}

	names, err := m.testDatabaseNames(db)
	if err != nil {
		return err
	}

	return m.dropDatabasesNamed(db, names)
}

func (m MariaDBHelper) deletePermissionsToCreateTestDatabases(db *sql.DB) error {
	_, err := db.Exec(`DELETE FROM mysql.db WHERE Db IN('test', 'test\_%')`)
	if err != nil {
		m.logger.Error("error deleting permissions for test databases", err)
		return err
	}

	return nil
}

func (m MariaDBHelper) testDatabaseNames(db *sql.DB) ([]string, error) {
	var allTestDatabaseNames []string
	testDatabaseNames, err := m.showDatabaseNamesLike("test", db)
	if err != nil {
		m.logger.Error("error searching for 'test' databases", err)
		return nil, err
	}
	allTestDatabaseNames = append(allTestDatabaseNames, testDatabaseNames...)

	testUnderscoreDatabaseNames, err := m.showDatabaseNamesLike(`test\_%`, db)
	if err != nil {
		m.logger.Error("error searching for 'test_%' databases", err)
		return nil, err
	}
	allTestDatabaseNames = append(allTestDatabaseNames, testUnderscoreDatabaseNames...)
	return allTestDatabaseNames, nil
}

func (m MariaDBHelper) showDatabaseNamesLike(pattern string, db *sql.DB) ([]string, error) {
	rows, err := db.Query(fmt.Sprintf("SHOW DATABASES LIKE '%s'", pattern))
	if err != nil {
		return nil, err
	}

	var dbNames []string
	defer rows.Close()
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		if err != nil {
			return nil, err
		}

		dbNames = append(dbNames, name)
	}
	err = rows.Err() // get any error encountered during iteration
	if err != nil {
		return nil, err
	}

	return dbNames, nil
}

func (m MariaDBHelper) dropDatabasesNamed(db *sql.DB, names []string) error {
	for _, n := range names {
		_, err := db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", n))
		if err != nil {
			return err
		}
	}

	return nil
}
