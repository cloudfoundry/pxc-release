package seeder

import (
	"database/sql"
	"fmt"

	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry/galera-init/config"
	_ "github.com/go-sql-driver/mysql"
)

//go:generate counterfeiter . Seeder

type Seeder interface {
	CreateDBIfNeeded() error
	IsExistingUser() (bool, error)
	CreateUser() error
	UpdateUser() error
	GrantUserPrivileges() error
}

type seeder struct {
	db            *sql.DB
	dbSkipBinLogs *sql.DB
	config        config.PreseededDatabase
	logger        lager.Logger
}

func NewSeeder(db *sql.DB, dbSkipBinLogs *sql.DB, config config.PreseededDatabase, logger lager.Logger) Seeder {
	return &seeder{
		db:            db,
		dbSkipBinLogs: dbSkipBinLogs,
		config:        config,
		logger:        logger,
	}
}

func (s seeder) CreateDBIfNeeded() error {
	_, err := s.db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", s.config.DBName))
	if err != nil {
		s.logger.Error("Error creating preseeded database", err, lager.Data{"dbName": s.config.DBName})
		return err
	}
	return nil
}

func (s seeder) IsExistingUser() (bool, error) {
	rows, err := s.dbSkipBinLogs.Query(fmt.Sprintf(
		"SELECT User FROM mysql.user WHERE User = '%s'",
		s.config.User))
	if err != nil {
		s.logger.Error("Error getting list of users", err, lager.Data{
			"dbName": s.config.DBName,
		})
		return false, err
	}

	return rows.Next(), nil
}

func (s seeder) CreateUser() error {
	_, err := s.dbSkipBinLogs.Exec(fmt.Sprintf(
		"CREATE USER `%s` IDENTIFIED BY '%s'",
		s.config.User,
		s.config.Password))
	if err != nil {
		s.logger.Error("Error creating user", err, lager.Data{
			"user": s.config.User,
		})
		return err
	}
	return nil
}

func (s seeder) UpdateUser() error {
	_, err := s.dbSkipBinLogs.Exec(fmt.Sprintf(
		"SET PASSWORD FOR `%s` = PASSWORD('%s')",
		s.config.User,
		s.config.Password,
	))
	if err != nil {
		s.logger.Error("Error updating user", err, lager.Data{
			"user": s.config.User,
		})
		return err
	}
	return nil
}

func (s seeder) GrantUserPrivileges() error {
	_, err := s.dbSkipBinLogs.Exec(fmt.Sprintf(
		"GRANT ALL ON `%s`.* TO '%s'@'%%'",
		s.config.DBName,
		s.config.User))
	if err != nil {
		s.logger.Error("Error granting user privileges", err, lager.Data{
			"dbName": s.config.DBName,
			"user":   s.config.User,
		})
		return err
	}

	_, err = s.dbSkipBinLogs.Exec(fmt.Sprintf(
		"REVOKE LOCK TABLES ON `%s`.* FROM '%s'@'%%'",
		s.config.DBName,
		s.config.User,
	))
	if err != nil {
		s.logger.Error("Error revoking LOCK TABLES privilege", err, lager.Data{
			"dbName": s.config.DBName,
			"user":   s.config.User,
		})
		return err
	}

	return nil
}
