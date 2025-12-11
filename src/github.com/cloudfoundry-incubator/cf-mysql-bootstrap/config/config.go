package config

import (
	"flag"
	"fmt"
	"os"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagerflags"
	"gopkg.in/validator.v2"
	"gopkg.in/yaml.v3"
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

func defaultConfig() Config {
	return Config{
		ShutDownMysql:             "stop_mysql",
		MysqlStatus:               "mysql_status",
		GetSeqNumber:              "sequence_number",
		StartMysqlInJoinMode:      "start_mysql_join",
		StartMysqlInBootstrapMode: "start_mysql_bootstrap",
	}
}

func NewConfig(osArgs []string) (*Config, error) {
	var rootConfig Config

	binaryName := osArgs[0]
	configurationOptions := osArgs[1:]

	// Define command line flags
	flags := flag.NewFlagSet(binaryName, flag.ExitOnError)
	var configData = flags.String("config", "", "json encoded configuration string")
	var configPath = flags.String("configPath", "", "path to configuration file with json encoded content")

	lagerflags.AddFlags(flags)
	flags.Parse(configurationOptions)

	// Start with defaults
	rootConfig = defaultConfig()

	// Load configuration from command line, file, or environment
	err := loadConfig(&rootConfig, *configData, *configPath)
	if err != nil {
		return nil, err
	}

	rootConfig.Logger, _ = lagerflags.NewFromConfig(binaryName, lagerflags.ConfigFromFlags())
	return &rootConfig, nil
}

func (c Config) Validate() error {
	return validator.Validate(c)
}

// Load configuration from sources in order of precedence:
// command line data "-config" or file "-configPath", environment variables
// for data CONFIG or file CONFIG_PATH.
func loadConfig(config *Config, configData, configPath string) error {
	var yamlData []byte
	var err error

	if configData != "" {
		yamlData = []byte(configData)
	} else if configPath != "" {
		yamlData, err = os.ReadFile(configPath)
		if err != nil {
			return fmt.Errorf("reading config file %s: %w", configPath, err)
		}
	} else if envConfig := os.Getenv("CONFIG"); envConfig != "" {
		yamlData = []byte(envConfig)
	} else if envConfigPath := os.Getenv("CONFIG_PATH"); envConfigPath != "" {
		yamlData, err = os.ReadFile(envConfigPath)
		if err != nil {
			return fmt.Errorf("reading config file from CONFIG_PATH %s: %w", envConfigPath, err)
		}
	} else {
		return fmt.Errorf("no configuration provided: use -config, -configPath, CONFIG, or CONFIG_PATH")
	}

	// Parse YAML
	if err := yaml.Unmarshal(yamlData, config); err != nil {
		return fmt.Errorf("parsing YAML config: %w", err)
	}

	return nil
}
