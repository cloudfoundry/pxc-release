package main

import (
	"errors"
	"fmt"
	"os"

	"code.cloudfoundry.org/lager/v3"
	bootstrapperPkg "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper/node_manager"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/clock"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
)

func main() {
	rootConfig, err := config.NewConfig(os.Args)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse config and build logger: %s", err.Error()))
	}

	err = rootConfig.Validate()
	if err != nil {
		panic(fmt.Sprintf("Invalid config: %s", err.Error()))
	}

	logger := rootConfig.Logger

	nodeManager := node_manager.New(rootConfig, clock.DefaultClock())
	bootstrapper := bootstrapperPkg.New(nodeManager)

	var actionTaken bool
	if rootConfig.RepairMode == "bootstrap" {
		actionTaken, err = bootstrapper.Bootstrap()
	} else if rootConfig.RepairMode == "rejoin-unsafe" {
		actionTaken, err = bootstrapper.RejoinUnsafe()
	} else {
		logger.Error("Invalid repair mode:", errors.New(fmt.Sprintf("%s", rootConfig.RepairMode)))
		fmt.Printf("Invalid repair mode: %s", rootConfig.RepairMode)
		os.Exit(1)
	}

	if err != nil {
		logger.Error("Failed to repair cluster", err, lager.Data{
			"config": rootConfig,
		})
		printHumanReadableErr(err, rootConfig.RepairMode)
		os.Exit(1)
	}

	if !actionTaken {
		fmt.Println("No action taken - cluster already healthy")
		os.Exit(0)
	}

	logger.Info("Successfully repaired cluster")
	fmt.Println("Successfully repaired cluster")
}

func printHumanReadableErr(err error, mode string) {
	var docsLink string

	if mode == "bootstrap" {
		docsLink = "https://github.com/cloudfoundry/cf-mysql-release/blob/master/docs/bootstrapping.md"
	} else {
		docsLink = "https://github.com/cloudfoundry/cf-mysql-release/blob/master/docs/cluster-behavior.md#forcing-a-node-to-rejoin-the-cluster-unsafe-procedure"
	}

	fmt.Printf(`
		##################################################################################
		Error: %s

		Reference the docs for more information:
		%s
		##################################################################################
		`, err, docsLink)
}
