package main

import (
	"errors"
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
	if err != nil {
		panic(fmt.Sprintf("Failed to parse config: %s", err.Error()))
	}

	err = rootConfig.Validate()
	if err != nil {
		panic(fmt.Sprintf("Invalid config: %s", err.Error()))
	}

	err = rootConfig.BuildLogger()
	if err != nil {
		panic(fmt.Sprintf("Failed to build logger: %s", err.Error()))
	}
	logger := rootConfig.Logger

	nodeManager := node_manager.New(rootConfig, clock.DefaultClock())
	bootstrapper := bootstrapperPkg.New(nodeManager)

	if rootConfig.RepairMode == "bootstrap" {
		err = bootstrapper.Bootstrap()
	} else if rootConfig.RepairMode == "force-rejoin" {
		err = bootstrapper.ForceRejoin()
	} else {
		logger.Error("Invalid repair mode:", errors.New(fmt.Sprintf("%s", rootConfig.RepairMode)))
		printHumanReadableErr(err)
		os.Exit(1)
	}

	if err != nil {
		logger.Error("Failed to repair cluster", err, lager.Data{
			"config": rootConfig,
		})
		printHumanReadableErr(err)
		os.Exit(1)
	}

	logger.Info("Successfully repaired cluster")
	fmt.Println("Successfully repaired cluster")
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
