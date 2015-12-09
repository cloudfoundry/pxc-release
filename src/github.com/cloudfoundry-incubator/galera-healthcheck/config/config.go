package config

import (
	"errors"
	"flag"
	"fmt"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/pivotal-cf-experimental/service-config"
	"github.com/pivotal-golang/lager"
	"gopkg.in/validator.v2"
)

type Config struct {
	DB                    DBConfig    `validate:"nonzero"`
	Monit                 MonitConfig `validate:"nonzero"`
	Host                  string      `validate:"nonzero"`
	Port                  int         `validate:"nonzero"`
	ArbitratorNode        bool
	AvailableWhenDonor    bool
	AvailableWhenReadOnly bool
	PidFile               string `validate:"nonzero"`
	Logger                lager.Logger
	MysqldPath            string                  `validate:"nonzero"`
	BootstrapEndpoint     BootstrapEndpointConfig `validate:"nonzero"`
}

type DBConfig struct {
	Host     string `validate:"nonzero"`
	User     string `validate:"nonzero"`
	Port     int    `validate:"nonzero"`
	Password string `validate:"nonzero"`
}

type MonitConfig struct {
	Host               string `validate:"nonzero"`
	User               string `validate:"nonzero"`
	Port               int    `validate:"nonzero"`
	Password           string `validate:"nonzero"`
	MysqlStateFilePath string `validate:"nonzero"`
	ServiceName        string `validate:"nonzero"`
}

type BootstrapEndpointConfig struct {
	Username string `validate:"nonzero"`
	Password string `validate:"nonzero"`
}

func defaultConfig() *Config {
	var defaultConfig Config
	defaultConfig = Config{
		Host: "0.0.0.0",
		Port: 8080,
		DB: DBConfig{
			Host:     "0.0.0.0",
			Port:     3306,
			User:     "root",
			Password: "",
		},
		AvailableWhenDonor:    true,
		AvailableWhenReadOnly: false,
	}
	return &defaultConfig
}

func NewConfig(osArgs []string) (*Config, error) {
	var rootConfig Config

	binaryName := osArgs[0]
	configurationOptions := osArgs[1:]
	serviceConfig := service_config.New()
	flags := flag.NewFlagSet(binaryName, flag.ExitOnError)

	cf_lager.AddFlags(flags)

	serviceConfig.AddFlags(flags)
	serviceConfig.AddDefaults(defaultConfig())
	flags.Parse(configurationOptions)

	rootConfig.Logger, _ = cf_lager.New("Galera Healthcheck")

	err := serviceConfig.Read(&rootConfig)
	return &rootConfig, err
}

func (c Config) Validate() error {
	rootConfigErr := validator.Validate(c)
	var errString string
	if rootConfigErr != nil {
		errString = formatErrorString(rootConfigErr, "")
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
