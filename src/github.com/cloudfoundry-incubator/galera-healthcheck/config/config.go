package config

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"os"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagerflags"
	"code.cloudfoundry.org/tlsconfig"
	"github.com/pkg/errors"
	"gopkg.in/validator.v2"
	"gopkg.in/yaml.v3"
)

type Config struct {
	DB                    DBConfig              `yaml:"DB" validate:"nonzero"`
	Monit                 MonitConfig           `yaml:"Monit" validate:"nonzero"`
	Host                  string                `yaml:"Host" validate:"nonzero"`
	Port                  int                   `yaml:"Port" validate:"nonzero"`
	AvailableWhenDonor    bool                  `yaml:"AvailableWhenDonor"`
	AvailableWhenReadOnly bool                  `yaml:"AvailableWhenReadOnly"`
	MysqldPath            string                `yaml:"MysqldPath" validate:"nonzero"`
	MyCnfPath             string                `yaml:"MyCnfPath" validate:"nonzero"`
	SidecarEndpoint       SidecarEndpointConfig `yaml:"SidecarEndpoint" validate:"nonzero"`
	Logger                lager.Logger          `yaml:"-"`
}

type DBConfig struct {
	User     string `yaml:"User,omitempty" validate:"nonzero"`
	Password string `yaml:"Password" validate:"nonzero"`
	Socket   string `yaml:"Socket,omitempty" validate:"nonzero"`
}

type MonitConfig struct {
	Host                          string `yaml:"Host" validate:"nonzero"`
	User                          string `yaml:"User" validate:"nonzero"`
	Port                          string `yaml:"Port" validate:"nonzero"`
	Password                      string `yaml:"Password" validate:"nonzero"`
	MysqlStateFilePath            string `yaml:"MysqlStateFilePath"`
	ServiceName                   string `yaml:"ServiceName" validate:"nonzero"`
	GaleraInitStatusServerAddress string `yaml:"GaleraInitStatusServerAddress" validate:"nonzero"`
}

type SidecarEndpointConfig struct {
	Username string      `yaml:"Username" validate:"nonzero"`
	Password string      `yaml:"Password" validate:"nonzero"`
	TLS      EndpointTLS `yaml:"TLS"`
}

type EndpointTLS struct {
	Enabled     bool   `yaml:"Enabled"`
	Certificate string `yaml:"Certificate"`
	PrivateKey  string `yaml:"PrivateKey"`
}

func defaultConfig() Config {
	return Config{
		Host: "0.0.0.0",
		Port: 8080,
		DB: DBConfig{
			Socket:   "/var/vcap/sys/run/pxc-mysql/mysqld.sock",
			User:     "root",
			Password: "",
		},
		AvailableWhenDonor:    true,
		AvailableWhenReadOnly: false,
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

func (c *Config) NetworkListener() (net.Listener, error) {
	address := fmt.Sprintf("%s:%d", c.Host, c.Port)

	if !c.SidecarEndpoint.TLS.Enabled {
		return net.Listen("tcp", address)
	}

	serverCert, err := tls.X509KeyPair([]byte(c.SidecarEndpoint.TLS.Certificate), []byte(c.SidecarEndpoint.TLS.PrivateKey))
	if err != nil {
		return nil, err
	}

	tlsConfig, err := tlsconfig.Build(
		tlsconfig.WithInternalServiceDefaults(),
		tlsconfig.WithIdentity(serverCert),
	).Server()
	if err != nil {
		return nil, errors.Wrap(err, "generating tls config")
	}

	return tls.Listen("tcp", address, tlsConfig)
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
			return fmt.Errorf("failed to read config file %s: %w", configPath, err)
		}
	} else if envConfig := os.Getenv("CONFIG"); envConfig != "" {
		yamlData = []byte(envConfig)
	} else if envConfigPath := os.Getenv("CONFIG_PATH"); envConfigPath != "" {
		yamlData, err = os.ReadFile(envConfigPath)
		if err != nil {
			return fmt.Errorf("failed to read config file from CONFIG_PATH %s: %w", envConfigPath, err)
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
