package seeder

import (
	"fmt"

	"database/sql"

	"github.com/cloudfoundry/mariadb_ctrl/config"
	_ "github.com/go-sql-driver/mysql"
	"github.com/pivotal-golang/lager"
)

type Seeder struct {
	db     *sql.DB
	config config.PreseededDatabase
	logger lager.Logger
}

func NewSeeder(db *sql.DB, config config.PreseededDatabase, logger lager.Logger) *Seeder {
	return &Seeder{
		db:     db,
		config: config,
		logger: logger,
	}
}

func (s Seeder) CreateDBIfNeeded() error {
	_, err := s.db.Exec(fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", s.config.DBName))
	if err != nil {
		s.logger.Error("Error creating preseeded database", err, lager.Data{"dbName": s.config.DBName})
		return err
	}
	return nil
}

func (s Seeder) IsExistingUser() (bool, error) {
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

func (s Seeder) CreateUser() error {
	_, err := s.db.Exec(fmt.Sprintf(
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

func (s Seeder) GrantUserAllPrivileges() error {
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
