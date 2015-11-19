package main

import (
	"os"

	bootstrapperPkg "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/clock"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
	"github.com/pivotal-golang/lager"
)

func main() {
	// call stop_mysql endpoint
	rootConfig, err := config.NewConfig(os.Args)
	logger := rootConfig.Logger
	if err != nil {
		logger.Fatal("Failed to parse config", err, lager.Data{
			"config": rootConfig,
		})
	}

	bootstrapper := bootstrapperPkg.New(rootConfig, clock.DefaultClock())
	err = bootstrapper.Run()
	if err != nil {
		logger.Fatal("Failed to bootstrap cluster", err, lager.Data{
			"config": rootConfig,
		})
	}
}
