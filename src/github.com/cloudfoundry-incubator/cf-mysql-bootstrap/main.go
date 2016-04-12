package main

import (
	"fmt"
	"os"

	bootstrapperPkg "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper/node_manager"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/clock"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
	"github.com/pivotal-golang/lager"
)

func main() {
	rootConfig, err := config.NewConfig(os.Args)
	err = rootConfig.BuildLogger()
	logger := rootConfig.Logger

	if err != nil {
		logger.Fatal("Failed to parse config", err, lager.Data{
			"config": rootConfig,
		})
	}

	nodeManager := node_manager.New(rootConfig, clock.DefaultClock())
	bootstrapper := bootstrapperPkg.New(nodeManager)
	err = bootstrapper.Bootstrap()

	if err != nil {
		logger.Error("Failed to bootstrap cluster", err, lager.Data{
			"config": rootConfig,
		})
		printHumanReadableErr(err)
		os.Exit(1)
	}

	logger.Info("Successfully bootstrapped cluster")
	fmt.Println("Successfully bootstrapped cluster")
}

func printHumanReadableErr(err error) {
	fmt.Printf(`
		##################################################################################
		Error: %s

		Reference the docs for more information:
		https://github.com/cloudfoundry/cf-mysql-release/blob/master/docs/bootstrapping.md
		##################################################################################
		`, err)
}
