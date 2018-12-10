package main

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/cloudfoundry/galera-init/cluster_health_checker"
	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/db_helper"
	"github.com/cloudfoundry/galera-init/os_helper"
	"github.com/cloudfoundry/galera-init/start_manager"
	"github.com/cloudfoundry/galera-init/start_manager/node_starter"
	"github.com/cloudfoundry/galera-init/upgrader"
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

	cfg.Logger.Info("starting")

	if err := startManager.Execute(); err != nil {
		cfg.Logger.Info(err.Error())
		if e, ok := err.(*exec.ExitError); ok {
			if ws := e.Sys().(syscall.WaitStatus); ws.Signaled() {
				os.Exit(int(ws.Signal()))
			} else {
				os.Exit(ws.ExitStatus())
			}
		} else {
			os.Exit(1)
		}
	}
	cfg.Logger.Info("exited")
}

func managerSetup(cfg *config.Config) start_manager.StartManager {
	OsHelper := os_helper.NewImpl()

	DBHelper := db_helper.NewDBHelper(
		OsHelper,
		&cfg.Db,
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

	NodeStarter := node_starter.NewStarter(
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
