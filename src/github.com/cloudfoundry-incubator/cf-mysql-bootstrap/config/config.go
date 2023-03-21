package config

import (
	"flag"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagerflags"
	"github.com/pivotal-cf-experimental/service-config"
	"gopkg.in/validator.v2"
)

type Config struct {
	Logger                    lager.Logger
	HealthcheckURLs           []string   `yaml:"HealthcheckURLs" validate:"nonzero"`
	BackendTLS                BackendTLS `yaml:"BackendTLS"`
	Username                  string     `yaml:"Username" validate:"nonzero"`
	Password                  string     `yaml:"Password" validate:"nonzero"`
	ShutDownMysql             string
	MysqlStatus               string
	GetSeqNumber              string
	StartMysqlInJoinMode      string
	StartMysqlInBootstrapMode string
	RepairMode                string `yaml:"RepairMode" validate:"nonzero,regexp=^(bootstrap|rejoin-unsafe)$"`
}

type BackendTLS struct {
	Enabled            bool   `yaml:"Enabled"`
	ServerName         string `yaml:"ServerName"`
	CA                 string `yaml:"CA"`
	InsecureSkipVerify bool   `yaml:"InsecureSkipVerify"`
}

func defaultConfig() *Config {
	defaultConfig := Config{
		ShutDownMysql:             "stop_mysql",
		MysqlStatus:               "mysql_status",
		GetSeqNumber:              "sequence_number",
		StartMysqlInJoinMode:      "start_mysql_join",
		StartMysqlInBootstrapMode: "start_mysql_bootstrap",
	}
	return &defaultConfig
}

func NewConfig(osArgs []string) (*Config, error) {
	var rootConfig Config

	binaryName := osArgs[0]
	configurationOptions := osArgs[1:]
	serviceConfig := service_config.New()
	flags := flag.NewFlagSet(binaryName, flag.ExitOnError)

	lagerflags.AddFlags(flags)

	serviceConfig.AddFlags(flags)
	serviceConfig.AddDefaults(defaultConfig())
	flags.Parse(configurationOptions)

	err := serviceConfig.Read(&rootConfig)

	rootConfig.Logger, _ = lagerflags.NewFromConfig(binaryName, lagerflags.ConfigFromFlags())
	return &rootConfig, err
}

func (c Config) Validate() error {
	return validator.Validate(c)
}
