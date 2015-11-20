package main

import (
	"fmt"
	"os"

	bootstrapperPkg "github.com/cloudfoundry-incubator/cf-mysql-bootstrap/bootstrapper"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/clock"
	"github.com/cloudfoundry-incubator/cf-mysql-bootstrap/config"
	"github.com/pivotal-golang/lager"
)

func main() {
	rootConfig, err := config.NewConfig(os.Args)
	logger := rootConfig.Logger
	var msg string

	if err != nil {
		logger.Fatal("Failed to parse config", err, lager.Data{
			"config": rootConfig,
		})
	}

	bootstrapper := bootstrapperPkg.New(rootConfig, clock.DefaultClock())
	msg, err = bootstrapper.Run()

	//Print output of bootstrap (this is useful for when we quit gracefully
	//without bootstrapping
	fmt.Println(msg)

	if err != nil {
		logger.Fatal("Failed to bootstrap cluster", err, lager.Data{
			"config": rootConfig,
		})
	}
}
