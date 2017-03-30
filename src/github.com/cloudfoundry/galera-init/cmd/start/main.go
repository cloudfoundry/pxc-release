package main

import (
	"io/ioutil"
	"os"

	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_starter"
	"github.com/cloudfoundry/mariadb_ctrl/upgrader"
	"fmt"
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

	err = managerSetup(cfg)

	if err != nil {
		cfg.Logger.Info(err.Error())
		panic("manager setup failed")
	}
	err = writePidFile(cfg)
	if err != nil {
		panic("could not write pid")

		cfg.Logger.Fatal("Error writing pidfile", err, lager.Data{
			"PidFile": cfg.PidFile,
		})
	}

	cfg.Logger.Info("mariadb_ctrl started")

	//
	//err = deletePidFile(cfg)
	//if err != nil {
	//	cfg.Logger.Error("Error deleting pidfile", err, lager.Data{
	//		"pidfile": cfg.PidFile,
	//	})
	//}
	//
	//if processErr != nil {
	//	cfg.Logger.Fatal("Error starting mariadb_ctrl", processErr)
	//}
	//
	//cfg.Logger.Info("Process exited without error.")
}

func writePidFile(cfg *config.Config) error {
	cfg.Logger.Info("Copying child pid to parent pid", lager.Data{
		"childPidfile": cfg.ChildPidFile,
		"pidfile": cfg.PidFile,
	})
	pidAsByteArray,err :=ioutil.ReadFile(cfg.ChildPidFile)
	if err !=nil{
		panic(fmt.Sprintf("could not read pid file from %s",cfg.ChildPidFile))
	}
	return ioutil.WriteFile(cfg.PidFile, pidAsByteArray, 0644)
}



func deletePidFile(cfg *config.Config) error {
	cfg.Logger.Info("Deleting pidfile", lager.Data{
		"pidfile": cfg.PidFile,
	})
	return os.Remove(cfg.PidFile)
}

func managerSetup(cfg *config.Config)  error {
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

	err := NodeStartManager.Execute()
	if err != nil {
		cfg.Logger.Info("execute error")
		return err
	}

	//cmd, err := NodeStarter.GetMysqlCmd()
	//if err != nil {
	//	cfg.Logger.Info("GetMysqlCmderror")
	//	return -1, err
	//}
	return nil
	// runner := node_runner.NewRunner(NodeStartManager, cfg.Logger)

	// sigRunner := sigmon.New(runner, os.Kill)

	// return sigRunner
}


