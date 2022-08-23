package config

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net"

	"code.cloudfoundry.org/lager"
	"code.cloudfoundry.org/lager/lagerflags"
	"code.cloudfoundry.org/tlsconfig"
	"github.com/pivotal-cf-experimental/service-config"
	"github.com/pkg/errors"
	"gopkg.in/validator.v2"

	"github.com/cloudfoundry-incubator/galera-healthcheck/domain"
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
	User     string `yaml:"User" validate:"nonzero"`
	Password string `yaml:"Password" validate:"nonzero"`
	Socket   string `yaml:"Socket" validate:"nonzero"`
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

func defaultConfig() *Config {
	return &Config{
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
	serviceConfig := service_config.New()
	flags := flag.NewFlagSet(binaryName, flag.ExitOnError)

	lagerflags.AddFlags(flags)

	serviceConfig.AddFlags(flags)
	serviceConfig.AddDefaults(defaultConfig())
	flags.Parse(configurationOptions)

	rootConfig.Logger, _ = lagerflags.NewFromConfig(binaryName, lagerflags.ConfigFromFlags())

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

func (c *Config) IsHealthy(state domain.DBState) bool {
	if state.ReadOnly && !c.AvailableWhenReadOnly {
		return false
	}

	return (state.WsrepLocalState == domain.Synced) || (state.WsrepLocalState == domain.DonorDesynced && c.AvailableWhenDonor)
}
