package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"

	"github.com/cloudfoundry-incubator/cf-lager"
	"github.com/cloudfoundry/mariadb_ctrl/config"
	"github.com/cloudfoundry/mariadb_ctrl/preparer"
	"github.com/pivotal-cf-experimental/service-config"
	"github.com/pivotal-golang/lager"
	"github.com/tedsuo/ifrit"
)

func main() {

	var prestartFlag string
	var processErr error

	flags := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	serviceConfig := service_config.New()
	serviceConfig.AddFlags(flags)
	serviceConfig.AddDefaults(config.Config{
		Db: config.DBHelper{
			User: "root",
		},
	})
	cf_lager.AddFlags(flags)
	flags.StringVar(&prestartFlag, "prestart", "false", "Start mariadb_ctrl in prestart mode")

	flags.Parse(os.Args[1:])

	logger, _ := cf_lager.New("mariadb_ctrl")

	var rootConfig config.Config
	err := serviceConfig.Read(&rootConfig)
	if err != nil {
		logger.Fatal("Error reading config file", err)
	}

	if prestartFlag == "true" {
		rootConfig.Prestart = true
	} else {
		rootConfig.Prestart = false
	}

	err = rootConfig.Validate()
	if err != nil {
		logger.Fatal("Error validating config", err)
	}

	procSetup := preparer.New(logger, rootConfig)
	sigRunner := procSetup.Prepare()

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

	if prestartFlag == "true" {
		process.Signal(os.Kill)
		<-process.Wait()
	} else {
		processErr = <-process.Wait()
	}

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
