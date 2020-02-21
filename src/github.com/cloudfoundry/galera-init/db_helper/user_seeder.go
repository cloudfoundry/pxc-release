package db_helper

import (
	"database/sql"
	"errors"
	"fmt"

	"code.cloudfoundry.org/lager"
)

//go:generate counterfeiter . UserSeeder
type UserSeeder interface {
	SeedUser(username string, password string, host string, role string) error
}

type userSeeder struct {
	db     *sql.DB
	logger lager.Logger
}

func NewUserSeeder(db *sql.DB, logger lager.Logger) UserSeeder {
	return &userSeeder{
		db:     db,
		logger: logger,
	}
}

func (seeder userSeeder) SeedUser(user string, password string, host string, role string) error {
	roleQuery, err := getRoleQuery(role)
	if err != nil {
		seeder.logger.Error("Invalid role", err, lager.Data{
			"user": user,
			"role": role,
		})
		return err
	}

	hostString, err := getHostString(host)
	if err != nil {
		seeder.logger.Error("Invalid host", err, lager.Data{
			"user": user,
			"host": host,
		})
		return err
	}

	_, err = seeder.db.Exec(fmt.Sprintf(
		"CREATE USER IF NOT EXISTS `%s`@`%s` IDENTIFIED BY '%s'",
		user,
		hostString,
		password))
	if err != nil {
		seeder.logger.Error("Error creating user", err, lager.Data{
			"user": user,
		})
		return err
	}

	_, err = seeder.db.Exec(fmt.Sprintf(
		"ALTER USER `%s`@`%s` IDENTIFIED BY '%s'",
		user,
		hostString,
		password))
	if err != nil {
		seeder.logger.Error("Error updating user password", err, lager.Data{
			"user": user,
		})
		return err
	}

	_, err = seeder.db.Exec(fmt.Sprintf(
		roleQuery,
		user,
		hostString))
	if err != nil {
		seeder.logger.Error("Error changing grants on user", err, lager.Data{
			"user": user,
		})
		return err
	}
	return nil
}

func getRoleQuery(role string) (string, error) {
	if role == "admin" {
		return "GRANT ALL PRIVILEGES ON *.* TO `%s`@`%s` WITH GRANT OPTION", nil
	} else if role == "minimal" {
		return "REVOKE ALL PRIVILEGES ON *.* FROM `%s`@`%s`", nil
	}
	return "", errors.New(fmt.Sprintf("Invalid role: %s", role))
}

func getHostString(host string) (string, error) {
	switch host {
	case "localhost":
		return "localhost", nil
	case "loopback":
		return "127.0.0.1", nil
	case "any":
		return "%", nil
	default:
		return "", errors.New(fmt.Sprintf("Invalid host: %s", host))
	}
}
