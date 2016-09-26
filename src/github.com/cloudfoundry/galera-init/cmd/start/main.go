package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	"code.cloudfoundry.org/cflager"
	"code.cloudfoundry.org/lager"
	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_runner"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager/node_starter"
	"github.com/cloudfoundry/mariadb_ctrl/upgrader"
	"github.com/pivotal-cf-experimental/service-config"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/sigmon"
)

func main() {
	var processErr error

	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	serviceConfig := service_config.New()
	serviceConfig.AddFlags(flags)
	serviceConfig.AddDefaults(config.Config{
		Db: config.DBHelper{
			User: "root",
		},
	})
	cflager.AddFlags(flags)

	flags.Parse(os.Args[1:])

	logger, _ := cflager.New("mariadb_ctrl")

	var rootConfig config.Config
	err := serviceConfig.Read(&rootConfig)
	if err != nil {
		logger.Fatal("Error reading config file", err)
	}

	err = rootConfig.Validate()
	if err != nil {
		logger.Fatal("Error validating config", err)
	}

	sigRunner := newRunner(logger, rootConfig)

	process := ifrit.Background(sigRunner)

	select {
	case err = <-process.Wait():
		logger.Error("Error starting mysqld", err)
		os.Exit(1)
	case <-process.Ready():
		//continue
	}

	err = writePidFile(rootConfig, logger)
	if err != nil {
		process.Signal(os.Kill)
		<-process.Wait()

		logger.Fatal("Error writing pidfile", err, lager.Data{
			"PidFile": rootConfig.PidFile,
		})
	}

	logger.Info("mariadb_ctrl started")

	processErr = <-process.Wait()

	err = deletePidFile(rootConfig, logger)
	if err != nil {
		logger.Error("Error deleting pidfile", err, lager.Data{
			"pidfile": rootConfig.PidFile,
		})
	}

	if processErr != nil {
		logger.Fatal("Error starting mariadb_ctrl", processErr)
	}

	logger.Info("Process exited without error.")
}

func writePidFile(rootConfig config.Config, logger lager.Logger) error {
	logger.Info(fmt.Sprintf("Writing pid to %s", rootConfig.PidFile))
	return ioutil.WriteFile(rootConfig.PidFile, []byte(strconv.Itoa(os.Getpid())), 0644)
}

func deletePidFile(rootConfig config.Config, logger lager.Logger) error {
	logger.Info(fmt.Sprintf("Deleting pidfile: %s", rootConfig.PidFile))
	return os.Remove(rootConfig.PidFile)
}

func newRunner(logger lager.Logger, rootConfig config.Config) ifrit.Runner {
	OsHelper := os_helper.NewImpl()

	DBHelper := mariadb_helper.NewMariaDBHelper(
		OsHelper,
		rootConfig.Db,
		rootConfig.LogFileLocation,
		logger,
	)

	Upgrader := upgrader.NewUpgrader(
		OsHelper,
		rootConfig.Upgrader,
		logger,
		DBHelper,
	)

	ClusterHealthChecker := cluster_health_checker.NewClusterHealthChecker(
		rootConfig.Manager.ClusterIps,
		logger,
	)

	NodeStarter := node_starter.NewStarter(
		DBHelper,
		OsHelper,
		rootConfig.Manager,
		logger,
		ClusterHealthChecker,
	)

	NodeStartManager := start_manager.New(
		OsHelper,
		rootConfig.Manager,
		DBHelper,
		Upgrader,
		NodeStarter,
		logger,
		ClusterHealthChecker,
	)

	runner := node_runner.NewRunner(NodeStartManager, logger)

	sigRunner := sigmon.New(runner, os.Kill)

	return sigRunner
}
