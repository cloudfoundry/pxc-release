package config

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"time"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagerflags"
	"github.com/pivotal-cf-experimental/service-config"
	"gopkg.in/validator.v2"
)

type Config struct {
	LogFileLocation string       `yaml:"LogFileLocation" validate:"nonzero"`
	Db              DBHelper     `yaml:"Db"`
	Manager         StartManager `yaml:"Manager"`
	BackendTLS      BackendTLS   `yaml:"BackendTLS"`
	Logger          lager.Logger
}

type DBHelper struct {
	Password           string              `yaml:"Password"`
	PostStartSQLFiles  []string            `yaml:"PostStartSQLFiles"`
	PreseededDatabases []PreseededDatabase `yaml:"PreseededDatabases"`
	SeededUsers        []SeededUser        `yaml:"SeededUsers"`
	SkipBinlog         bool                `yaml:"SkipBinlog"`
	Socket             string              `yaml:"Socket"`
	User               string              `yaml:"User" validate:"nonzero"`
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

func NewConfig(osArgs []string) (*Config, error) {
	var c Config

	binaryName := osArgs[0]
	configurationOptions := osArgs[1:]

	serviceConfig := service_config.New()
	flags := flag.NewFlagSet(binaryName, flag.ExitOnError)

	lagerflags.AddFlags(flags)

	serviceConfig.AddFlags(flags)
	serviceConfig.AddDefaults(Config{
		Db: DBHelper{
			User: "root",
		},
		Manager: StartManager{
			GrastateFileLocation: "/var/vcap/store/pxc-mysql/grastate.dat",
		},
	})
	flags.Parse(configurationOptions)

	err := serviceConfig.Read(&c)

	c.Logger, _ = lagerflags.NewFromConfig(binaryName, lagerflags.ConfigFromFlags())

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

func (c *Config) HTTPClient() *http.Client {
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

func (c *Config) ClusterUrls() (urls []string) {
	for _, ip := range c.Manager.ClusterIps {
		urls = append(urls, "http://"+ip+":9200/", "https://"+ip+":9201/")
	}
	return urls
}
