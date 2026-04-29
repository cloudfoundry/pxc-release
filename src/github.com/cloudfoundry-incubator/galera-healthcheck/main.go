package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	"code.cloudfoundry.org/lager/v3"
	_ "github.com/go-sql-driver/mysql"

	"github.com/cloudfoundry-incubator/galera-healthcheck/api"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/galera_init_client"
	"github.com/cloudfoundry-incubator/galera-healthcheck/healthcheck"
	"github.com/cloudfoundry-incubator/galera-healthcheck/mysqld_cmd"
	"github.com/cloudfoundry-incubator/galera-healthcheck/node_manager"
	"github.com/cloudfoundry-incubator/galera-healthcheck/sequence_number"
)

func main() {
	rootConfig, err := config.NewConfig(os.Args)
	if err != nil {
		panic(fmt.Sprintf("Failed to parse config and build logger: %s", err.Error()))
	}

	logger := rootConfig.Logger

	err = rootConfig.Validate()
	if err != nil {
		logger.Fatal("Failed to validate config", err)
	}

	db, err := sql.Open("mysql",
		fmt.Sprintf("%s:%s@unix(%s)/",
			rootConfig.DB.User,
			rootConfig.DB.Password,
			rootConfig.DB.Socket))

	if err != nil {
		logger.Fatal("db-initialize", err, lager.Data{
			"dbSocket": rootConfig.DB.Socket,
			"dbUser":   rootConfig.DB.User,
		})
	} else {
		logger.Info("db-initialize", lager.Data{
			"dbSocket": rootConfig.DB.Socket,
			"dbUser":   rootConfig.DB.User,
		})
	}

	var mysqlProcessMutex sync.Mutex
	mysqldCmd := mysqld_cmd.NewMysqldCmd(logger, *rootConfig)

	initClient, err := galera_init_client.NewClientForAddress(
		rootConfig.GaleraInit.GaleraInitStatusServerAddress,
		2*time.Minute,
	)
	if err != nil {
		logger.Fatal("galera-init-client", err, lager.Data{
			"address": rootConfig.GaleraInit.GaleraInitStatusServerAddress,
		})
	}
	serviceManager := &node_manager.NodeManager{
		ServiceName:   rootConfig.GaleraInit.ServiceName,
		StateFilePath: rootConfig.GaleraInit.MysqlStateFilePath,
		MonitClient:   initClient,
		Logger:        logger,
		Mutex:         &mysqlProcessMutex,
	}

	healthchecker := healthcheck.New(db, *rootConfig, logger)
	sequenceNumberchecker := sequence_number.New(db, mysqldCmd, *rootConfig, logger, &mysqlProcessMutex)
	stateSnapshotter := &healthcheck.DBStateSnapshotter{DB: db}

	router, err := api.NewRouter(
		logger,
		rootConfig,
		serviceManager,
		sequenceNumberchecker,
		healthchecker,
		stateSnapshotter,
	)
	if err != nil {
		logger.Fatal("Failed to create router", err)
	}

	address := fmt.Sprintf("%s:%d", rootConfig.Host, rootConfig.Port)
	listener, err := rootConfig.NetworkListener()
	if err != nil {
		logger.Fatal("tcp-listen", err, lager.Data{
			"address": address,
		})
	}

	url := fmt.Sprintf("https://%s/", address)
	logger.Info("Serving healthcheck endpoint", lager.Data{
		"url": url,
	})

	if err := http.Serve(listener, router); err != nil {
		logger.Fatal("http-server", err)
	}
	logger.Info("graceful-exit")
}
