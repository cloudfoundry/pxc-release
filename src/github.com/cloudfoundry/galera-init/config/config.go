package config

import (
	"errors"
	"fmt"

	"github.com/pivotal-cf-experimental/service-config"
	"gopkg.in/validator.v2"

	"flag"

	"code.cloudfoundry.org/cflager"
	"code.cloudfoundry.org/lager"
)

type Config struct {
	LogFileLocation string       `yaml:"LogFileLocation" validate:"nonzero"`
	PidFile         string       `yaml:"PidFile" validate:"nonzero"`
	Db              DBHelper     `yaml:"Db"`
	Manager         StartManager `yaml:"Manager"`
	Upgrader        Upgrader     `yaml:"Upgrader"`
	Prestart        bool
	Logger          lager.Logger
}

type DBHelper struct {
	DaemonPath          string              `yaml:"DaemonPath" validate:"nonzero"`
	UpgradePath         string              `yaml:"UpgradePath" validate:"nonzero"`
	User                string              `yaml:"User" validate:"nonzero"`
	Password            string              `yaml:"Password"`
	ReadOnlyUserEnabled bool                `yaml:"ReadOnlyUserEnabled"`
	ReadOnlyUser        string              `yaml:"ReadOnlyUser" validate:"nonzero"`
	ReadOnlyPassword    string              `yaml:"ReadOnlyPassword"`
	PreseededDatabases  []PreseededDatabase `yaml:"PreseededDatabases"`
	PostStartSQLFiles   []string            `yaml:"PostStartSQLFiles"`
}

type StartManager struct {
	StateFileLocation string   `yaml:"StateFileLocation" validate:"nonzero"`
	ClusterIps        []string `yaml:"ClusterIps" validate:"nonzero"`
	MyIP              string   `yaml:"MyIP" validate:"nonzero"`
}

type Upgrader struct {
	PackageVersionFile      string `yaml:"PackageVersionFile" validate:"nonzero"`
	LastUpgradedVersionFile string `yaml:"LastUpgradedVersionFile" validate:"nonzero"`
}

type PreseededDatabase struct {
	DBName   string `yaml:"DBName" validate:"nonzero"`
	User     string `yaml:"User" validate:"nonzero"`
	Password string `yaml:"Password"`
}

func NewConfig(osArgs []string) (*Config, error) {
	var c Config

	binaryName := osArgs[0]
	configurationOptions := osArgs[1:]

	serviceConfig := service_config.New()
	flags := flag.NewFlagSet(binaryName, flag.ExitOnError)
	serviceConfig.AddFlags(flags)
	serviceConfig.AddDefaults(Config{
		Db: DBHelper{
			User: "root",
		},
	})

	cflager.AddFlags(flags)

	serviceConfig.AddFlags(flags)
	flags.Parse(configurationOptions)

	err := serviceConfig.Read(&c)

	c.Logger, _ = cflager.New(binaryName)

	return &c, err
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
