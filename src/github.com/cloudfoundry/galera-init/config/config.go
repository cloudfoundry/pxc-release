package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagerflags"
	"gopkg.in/validator.v2"
	"gopkg.in/yaml.v3"
)

type Config struct {
	LogFileLocation string       `yaml:"LogFileLocation" validate:"nonzero"`
	Db              DBHelper     `yaml:"Db"`
	Manager         StartManager `yaml:"Manager"`
	BackendTLS      BackendTLS   `yaml:"BackendTLS"`
	Logger          lager.Logger
}

type DBHelper struct {
	Password   string `yaml:"Password"`
	SkipBinlog bool   `yaml:"SkipBinlog"`
	Socket     string `yaml:"Socket"`
	User       string `yaml:"User" validate:"nonzero"`
}

type StartManager struct {
	StateFileLocation             string `yaml:"StateFileLocation" validate:"nonzero"`
	GrastateFileLocation          string
	ClusterIps                    []string `yaml:"ClusterIps" validate:"nonzero"`
	BootstrapNode                 bool     `yaml:"BootstrapNode"`
	ClusterProbeTimeout           int      `yaml:"ClusterProbeTimeout" validate:"nonzero"`
	GaleraInitStatusServerAddress string   `yaml:"GaleraInitStatusServerAddress" validate:"nonzero"`
}

type BackendTLS struct {
	Enabled    bool   `yaml:"Enabled"`
	ServerName string `yaml:"ServerName"`
	CA         string `yaml:"CA"`
}

type PreseededDatabase struct {
	DBName   string `yaml:"DBName" validate:"nonzero"`
	User     string `yaml:"User" validate:"nonzero"`
	Password string `yaml:"Password"`
}

type SeededUser struct {
	User     string `yaml:"User" validate:"nonzero"`
	Password string `yaml:"Password" validate:"nonzero"`
	Host     string `yaml:"Host" validate:"nonzero"`
	Role     string `yaml:"Role" validate:"nonzero"`
}

func defaultConfig() Config {
	return Config{
		Db: DBHelper{
			User: "root",
		},
		Manager: StartManager{
			GrastateFileLocation: "/var/vcap/store/pxc-mysql/grastate.dat",
		},
	}
}

func NewConfig(osArgs []string) (*Config, error) {
	var c Config

	binaryName := osArgs[0]
	configurationOptions := osArgs[1:]

	// Define command line flags
	flags := flag.NewFlagSet(binaryName, flag.ExitOnError)
	var configData = flags.String("config", "", "json encoded configuration string")
	var configPath = flags.String("configPath", "", "path to configuration file with json encoded content")

	lagerflags.AddFlags(flags)
	flags.Parse(configurationOptions)

	// Start with defaults
	c = defaultConfig()

	// Load configuration from command line, file, or environment
	err := loadConfig(&c, *configData, *configPath)
	if err != nil {
		return nil, err
	}

	c.Logger, _ = lagerflags.NewFromConfig(binaryName, lagerflags.ConfigFromFlags())

	return &c, nil
}

func (c Config) Validate() error {
	errString := ""
	err := validator.Validate(c)

	if err != nil {
		errString += formatErrorString(err, "")
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

func (c Config) HTTPClient() *http.Client {
	httpClient := &http.Client{
		Timeout: time.Duration(c.Manager.ClusterProbeTimeout) * time.Second,
	}

	if c.BackendTLS.Enabled {
		certPool := x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM([]byte(c.BackendTLS.CA)); !ok {
			// TODO: should we handle the failure parsing a CA?
		}

		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    certPool,
				ServerName: c.BackendTLS.ServerName,
			},
		}
	}

	return httpClient
}

func (c Config) ClusterUrls() (urls []string) {
	for _, ip := range c.Manager.ClusterIps {
		urls = append(urls, "http://"+ip+":9200/", "https://"+ip+":9201/")
	}
	return urls
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
