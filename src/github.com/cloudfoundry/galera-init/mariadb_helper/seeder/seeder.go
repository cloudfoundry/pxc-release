package seeder

import (
	"fmt"

	"database/sql"

	"github.com/cloudfoundry/mariadb_ctrl/config"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pivotal-golang/lager"
)

//go:generate counterfeiter . Seeder

type Seeder interface {
	CreateDBIfNeeded() error
	IsExistingUser() (bool, error)
	CreateUserForDB() error
	CreateUser(string, string) error
	GrantUserAllPrivileges() error
	GrantUserSuperROPrivileges(string) error
}

type seeder struct {
	db     *sql.DB
	config config.PreseededDatabase
	logger lager.Logger
}

func NewSeeder(db *sql.DB, config config.PreseededDatabase, logger lager.Logger) Seeder {
	return &seeder{
		db:     db,
		config: config,
		logger: logger,
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
	rows, err := s.db.Query(fmt.Sprintf(
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

func (s seeder) CreateUserForDB() error {
	err := s.CreateUser(s.config.User, s.config.Password)
	if err != nil {
		s.logger.Error("Error creating user", err, lager.Data{
			"user": s.config.User,
		})
		return err
	}
	return nil
}

func (s seeder) CreateUser(username, password string) error {
	_, err := s.db.Exec(fmt.Sprintf(
		"CREATE USER `%s` IDENTIFIED BY '%s'",
		username,
		password))
	if err != nil {
		s.logger.Error("Error creating user", err, lager.Data{
			"user": username,
		})
		return err
	}
	return nil
}

func (s seeder) GrantUserAllPrivileges() error {
	_, err := s.db.Exec(fmt.Sprintf(
		"GRANT ALL ON `%s`.* TO `%s`",
		s.config.DBName,
		s.config.User))
	if err != nil {
		s.logger.Error("Error granting user privileges", err, lager.Data{
			"dbName": s.config.DBName,
			"user":   s.config.User,
		})
		return err
	}
	return nil
}

func (s seeder) GrantUserSuperROPrivileges(username string) error {
	_, err := s.db.Exec(fmt.Sprintf(
		"GRANT SELECT ON `%s`.* TO `%s`",
		"*",
		username))
	if err != nil {
		s.logger.Error("Error granting super-user RO privileges", err, lager.Data{
			"user": username,
		})
		return err
	}
	return nil
}
