package main

import (
	"os"

	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_starter"
	"github.com/cloudfoundry/mariadb_ctrl/upgrader"
)

func main() {
	cfg, err := config.NewConfig(os.Args)
	if err != nil {
		cfg.Logger.Fatal("Error creating config", err)
		return
	}

	err = cfg.Validate()
	if err != nil {
		cfg.Logger.Fatal("Error validating config", err)
		return
	}

	startManager := managerSetup(cfg)
	err = managerStart(startManager)

	if err != nil {
		cfg.Logger.Info(err.Error())
		panic("manager start failed")
	}

	managerStop(startManager)

	cfg.Logger.Info("Process exited without error.")
}

func managerSetup(cfg *config.Config) start_manager.StartManager {
	OsHelper := os_helper.NewImpl()

	DBHelper := mariadb_helper.NewMariaDBHelper(
		OsHelper,
		cfg.Db,
		cfg.LogFileLocation,
		cfg.Logger,
	)

	Upgrader := upgrader.NewUpgrader(
		OsHelper,
		cfg.Upgrader,
		cfg.Logger,
		DBHelper,
	)

	ClusterHealthChecker := cluster_health_checker.NewClusterHealthChecker(
		cfg.Manager.ClusterIps,
		cfg.Manager.ClusterProbeTimeout,
		cfg.Logger,
	)

	NodeStarter := node_starter.NewPreStarter(
		DBHelper,
		OsHelper,
		cfg.Manager,
		cfg.Logger,
		ClusterHealthChecker,
	)

	NodeStartManager := start_manager.New(
		OsHelper,
		cfg.Manager,
		DBHelper,
		Upgrader,
		NodeStarter,
		cfg.Logger,
		ClusterHealthChecker,
	)

	return NodeStartManager
}

func managerStart(startManager start_manager.StartManager) error {
	return startManager.Execute()

}

func managerStop(startManager start_manager.StartManager) {
	startManager.Shutdown()

}
