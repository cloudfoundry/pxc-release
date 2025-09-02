package main

import (
	"database/sql"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"code.cloudfoundry.org/lager/v3"
	_ "github.com/go-sql-driver/mysql"

	"github.com/cloudfoundry-incubator/galera-healthcheck/api"
	"github.com/cloudfoundry-incubator/galera-healthcheck/config"
	"github.com/cloudfoundry-incubator/galera-healthcheck/healthcheck"
	"github.com/cloudfoundry-incubator/galera-healthcheck/monit_client"
	"github.com/cloudfoundry-incubator/galera-healthcheck/mysqld_cmd"
	"github.com/cloudfoundry-incubator/galera-healthcheck/node_manager"
	"github.com/cloudfoundry-incubator/galera-healthcheck/sequence_number"
)

func main() {
	rootConfig, err := config.NewConfig(os.Args)

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
	serviceManager := &node_manager.NodeManager{
		ServiceName:   rootConfig.Monit.ServiceName,
		StateFilePath: rootConfig.Monit.MysqlStateFilePath,
		MonitClient: monit_client.NewClient(
			net.JoinHostPort(rootConfig.Monit.Host, rootConfig.Monit.Port),
			rootConfig.Monit.User,
			rootConfig.Monit.Password,
			2*time.Minute,
		),
		GaleraInitAddress: rootConfig.Monit.GaleraInitStatusServerAddress,
		Logger:            logger,
		Mutex:             &mysqlProcessMutex,
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
