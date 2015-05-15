package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry/mariadb_ctrl/cluster_health_checker"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/mariadb_helper"
	"github.com/cloudfoundry/mariadb_ctrl/os_helper"
	"github.com/cloudfoundry/mariadb_ctrl/start_manager"
	"github.com/cloudfoundry/mariadb_ctrl/upgrader"
	"github.com/pivotal-cf-experimental/service-config"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/sigmon"
)

func main() {
	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	serviceConfig := service_config.New()
	serviceConfig.AddFlags(flags)

	serviceConfig.AddDefaults(config.Config{
		Db: config.DBHelper{
			User: "root",
		},
		Manager: config.StartManager{
			MaxDatabaseSeedTries: 1,
		},
	})
	cf_lager.AddFlags(flags)
	flags.Parse(os.Args[1:])

	logger, _ := cf_lager.New("mariadb_ctrl")

	var rootConfig config.Config
	err := serviceConfig.Read(&rootConfig)
	if err != nil {
		logger.Fatal("Error reading config file", err)
	}

	err = rootConfig.Validate()
	if err != nil {
		logger.Fatal("Error validating config", err)
	}

	osHelper := os_helper.NewImpl()

	mariaDBHelper := mariadb_helper.NewMariaDBHelper(
		osHelper,
		rootConfig.Db,
		rootConfig.LogFileLocation,
		logger,
	)

	upgrader := upgrader.NewUpgrader(
		osHelper,
		rootConfig.Upgrader,
		logger,
		mariaDBHelper,
	)

	galeraHelper := cluster_health_checker.NewClusterHealthChecker(
		rootConfig.Manager.ClusterIps,
		logger,
	)

	mgr := start_manager.New(
		osHelper,
		rootConfig.Manager,
		mariaDBHelper,
		upgrader,
		logger,
		galeraHelper,
	)

	runner := start_manager.NewRunner(mgr, logger)
	sigRunner := sigmon.New(runner, os.Kill)
	process := ifrit.Background(sigRunner)

	select {
	case err = <-process.Wait():
		logger.Fatal("Error starting mariadb", err)
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

	processErr := <-process.Wait()

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
