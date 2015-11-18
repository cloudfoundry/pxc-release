package config

import (
	"flag"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/pivotal-cf-experimental/service-config"
	"github.com/pivotal-golang/lager"
	"gopkg.in/validator.v2"
)

type Config struct {
	Logger                    lager.Logger
	HealthcheckURLs           []string `validate:"nonzero"`
	ShutDownMysql             string
	MysqlStatus               string
	GetSeqNumber              string
	StartMysqlInJoinMode      string
	StartMysqlInBootstrapMode string
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

	cf_lager.AddFlags(flags)

	serviceConfig.AddFlags(flags)
	serviceConfig.AddDefaults(defaultConfig())
	flags.Parse(configurationOptions)

	rootConfig.Logger, _ = cf_lager.New("BootStrap Errand")

	err := serviceConfig.Read(&rootConfig)
	return &rootConfig, err
}

func (c Config) Validate() error {
	return validator.Validate(c)
}
