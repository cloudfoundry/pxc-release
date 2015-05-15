package config

import (
	"errors"
	"fmt"

	"gopkg.in/validator.v2"
)

type Config struct {
	LogFileLocation string `validate:"nonzero"`
	PidFile         string `validate:"nonzero"`
	Db              DBHelper
	Manager         StartManager
	Upgrader        Upgrader
}

type DBHelper struct {
	DaemonPath         string `validate:"nonzero"`
	UpgradePath        string `validate:"nonzero"`
	User               string `validate:"nonzero"`
	Password           string
	PreseededDatabases []PreseededDatabase
}

type StartManager struct {
	StateFileLocation    string   `validate:"nonzero"`
	MaxDatabaseSeedTries int      `validate:"nonzero"`
	ClusterIps           []string `validate:"nonzero"`
	MyIP                 string   `validate:"nonzero"`
}

type Upgrader struct {
	PackageVersionFile      string `validate:"nonzero"`
	LastUpgradedVersionFile string `validate:"nonzero"`
}

type PreseededDatabase struct {
	DBName   string `validate:"nonzero"`
	User     string `validate:"nonzero"`
	Password string
}

func (c Config) Validate() error {
	errString := ""
	err := validator.Validate(c)

	if err != nil {
		errString += formatErrorString(err, "")
	}

	for i, db := range c.Db.PreseededDatabases {
		dbErr := validator.Validate(db)
		if dbErr != nil {
			errString += formatErrorString(
				dbErr,
				fmt.Sprintf("Db.PreseededDatabases[%d].", i),
			)
		}
	}

	if len(errString) > 0 {
		return errors.New(fmt.Sprintf("Validation errors: %s\n", errString))
	}

	return nil
}

func formatErrorString(err error, keyPrefix string) string {
	errs := err.(validator.ErrorMap)
	var errsString string
	for fieldName, validationMessage := range errs {
		errsString += fmt.Sprintf("%s%s : %s\n", keyPrefix, fieldName, validationMessage)
	}
	return errsString
}
