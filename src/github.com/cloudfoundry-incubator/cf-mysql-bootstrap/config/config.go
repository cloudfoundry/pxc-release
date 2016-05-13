package config

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/pivotal-cf-experimental/service-config"
	"github.com/pivotal-golang/lager"
	"gopkg.in/validator.v2"
)

type Config struct {
	Logger                    lager.Logger
	HealthcheckURLs           []string `yaml:"HealthcheckURLs" validate:"nonzero"`
	Username                  string   `yaml:"Username" validate:"nonzero"`
	Password                  string   `yaml:"Password" validate:"nonzero"`
	ShutDownMysql             string
	MysqlStatus               string
	GetSeqNumber              string
	StartMysqlInJoinMode      string
	StartMysqlInBootstrapMode string
	LogFilePath               string `yaml:"LogFilePath" validate:"nonzero"`
	RepairMode                string `yaml:"RepairMode" validate:"nonzero,regexp=^(bootstrap|force-rejoin)$"`
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

	serviceConfig.AddFlags(flags)
	serviceConfig.AddDefaults(defaultConfig())
	flags.Parse(configurationOptions)

	err := serviceConfig.Read(&rootConfig)
	return &rootConfig, err
}

func (c *Config) BuildLogger() error {
	logFileHandle, err := os.Create(c.LogFilePath)
	if err != nil {
		return errors.New(fmt.Sprintf("Could not open handle to logfile %s", err.Error()))
	}

	c.Logger = lager.NewLogger("BootStrap Errand")
	writerSink := lager.NewWriterSink(logFileHandle, lager.DEBUG)
	c.Logger.RegisterSink(writerSink)
	return nil
}

func (c Config) Validate() error {
	return validator.Validate(c)
}
