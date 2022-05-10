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
	Proxy      Proxy        `yaml:"Proxy" validate:"nonzero"`
	API        API          `yaml:"API" validate:"nonzero"`
	StaticDir  string       `yaml:"StaticDir" validate:"nonzero"`
	HealthPort uint         `yaml:"HealthPort" validate:"nonzero"`
	BackendTLS BackendTLS   `yaml:"BackendTLS"`
	Logger     lager.Logger `yaml:"-"`
}

type BackendTLS struct {
	Enabled    bool   `yaml:"Enabled"`
	ServerName string `yaml:"ServerName"`
	CA         string `yaml:"CA"`
}

type Proxy struct {
	Port                     uint      `yaml:"Port" validate:"nonzero"`
	InactiveMysqlPort        uint      `yaml:"InactiveMysqlPort"`
	Backends                 []Backend `yaml:"Backends" validate:"min=1"`
	HealthcheckTimeoutMillis uint      `yaml:"HealthcheckTimeoutMillis" validate:"nonzero"`
	ShutdownDelaySeconds     uint      `yaml:"ShutdownDelaySeconds"`
}

type API struct {
	Port           uint     `yaml:"Port" validate:"nonzero"`
	AggregatorPort uint     `yaml:"AggregatorPort" validate:"nonzero"`
	Username       string   `yaml:"Username" validate:"nonzero"`
	Password       string   `yaml:"Password" validate:"nonzero"`
	ForceHttps     bool     `yaml:"ForceHttps"`
	ProxyURIs      []string `yaml:"ProxyURIs"`
}

type Backend struct {
	Host           string `yaml:"Host" validate:"nonzero"`
	Port           uint   `yaml:"Port" validate:"nonzero"`
	StatusPort     uint   `yaml:"StatusPort" validate:"nonzero"`
	StatusEndpoint string `yaml:"StatusEndpoint" validate:"nonzero"`
	Name           string `yaml:"Name" validate:"nonzero"`
}

func (p Proxy) HealthcheckTimeout() time.Duration {
	return time.Duration(p.HealthcheckTimeoutMillis) * time.Millisecond
}

func (p Proxy) ShutdownDelay() time.Duration {
	return time.Duration(p.ShutdownDelaySeconds) * time.Second
}

func NewConfig(osArgs []string) (*Config, error) {
	var rootConfig Config

	binaryName := osArgs[0]
	configurationOptions := osArgs[1:]

	serviceConfig := service_config.New()
	flags := flag.NewFlagSet(binaryName, flag.ExitOnError)

	lagerflags.AddFlags(flags)

	serviceConfig.AddFlags(flags)
	flags.Parse(configurationOptions)

	err := serviceConfig.Read(&rootConfig)

	rootConfig.Logger, _ = lagerflags.NewFromConfig(binaryName, lagerflags.ConfigFromFlags())

	return &rootConfig, err
}

func (c Config) Validate() error {
	rootConfigErr := validator.Validate(c)
	var errString string
	if rootConfigErr != nil {
		errString = formatErrorString(rootConfigErr, "")
	}

	// validator.Validate does not work on nested arrays
	for i, backend := range c.Proxy.Backends {
		backendsErr := validator.Validate(backend)
		if backendsErr != nil {
			errString += formatErrorString(
				backendsErr,
				fmt.Sprintf("Proxy.Backends[%d].", i),
			)
		}
	}


	if c.BackendTLS.Enabled {
		certPool := x509.NewCertPool()
		if ok := certPool.AppendCertsFromPEM([]byte(c.BackendTLS.CA)); !ok {
			errString += fmt.Sprintf("%s%s : %s\n", "", "BackendTLS.CA", "Failed to Parse CA.")
		}
	}



	if len(errString) > 0 {
		return errors.New(fmt.Sprintf("Validation errors: %s\n", errString))
	}
	return nil
}

func (c *Config) HTTPClient() *http.Client {
	httpClient := &http.Client{
		Timeout: c.Proxy.HealthcheckTimeout(),
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

func formatErrorString(err error, keyPrefix string) string {
	errs := err.(validator.ErrorMap)
	var errsString string
	for fieldName, validationMessage := range errs {
		errsString += fmt.Sprintf("%s%s : %s\n", keyPrefix, fieldName, validationMessage)
	}
	return errsString
}
