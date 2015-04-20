package main

import (
	"flag"
	"os"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager"
	"github.com/cloudfoundry/mariadb_ctrl/upgrader"
	"github.com/pivotal-cf-experimental/service-config"
)

type Config struct {
	LogFileLocation string
	Db              mariadb_helper.Config
	Manager         start_manager.Config
	Upgrader        upgrader.Config
}

func main() {
	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	serviceConfig := service_config.New()
	serviceConfig.AddFlags(flags)

	serviceConfig.AddDefaults(Config{
		Db: mariadb_helper.Config{
			User: "root",
		},
		Manager: start_manager.Config{
			JobIndex:             1,
			MaxDatabaseSeedTries: 1,
		},
	})
	cf_lager.AddFlags(flags)
	flags.Parse(os.Args[1:])

	logger, _ := cf_lager.New("mariadb_ctrl")

	var config Config
	err := serviceConfig.Read(&config)
	if err != nil {
		logger.Fatal("Error reading config file", err)
	}

	osHelper := os_helper.NewImpl()

	mariaDBHelper := mariadb_helper.NewMariaDBHelper(
		osHelper,
		config.Db,
		config.LogFileLocation,
		logger,
	)

	upgrader := upgrader.NewUpgrader(
		osHelper,
		config.Upgrader,
		logger,
		mariaDBHelper,
	)

	galeraHelper := cluster_health_checker.NewClusterHealthChecker(
		config.Manager.ClusterIps,
		logger,
	)

	mgr := start_manager.New(
		osHelper,
		config.Manager,
		mariaDBHelper,
		upgrader,
		logger,
		galeraHelper,
	)

	err = mgr.Execute()
	if err != nil {
		logger.Fatal("Execution exited with an error", err)
	}
}
