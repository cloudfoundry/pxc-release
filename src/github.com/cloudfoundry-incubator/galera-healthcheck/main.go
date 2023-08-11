package main

import (
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

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

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	err = rootConfig.Validate()
	if err != nil {
		logger.Error("Failed to validate config", "error", err)
		os.Exit(1)
	}

	db, err := sql.Open("mysql",
		fmt.Sprintf("%s:%s@unix(%s)/",
			rootConfig.DB.User,
			rootConfig.DB.Password,
			rootConfig.DB.Socket))

	if err != nil {
		logger.Error("failed to initialize database connection pool",
			"error", err,
			"dbSocket", rootConfig.DB.Socket,
			"dbUser", rootConfig.DB.User,
		)
		os.Exit(1)
	} else {
		logger.Info("initialized database connection pool",
			"dbSocket", rootConfig.DB.Socket,
			"dbUser", rootConfig.DB.User,
		)
	}

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
	}

	healthchecker := healthcheck.New(db, *rootConfig)
	sequenceNumberchecker := sequence_number.New(db, mysqldCmd, *rootConfig, logger)
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
		logger.Error("Failed to initialize router", "error", err)
		os.Exit(1)
	}

	address := fmt.Sprintf("%s:%d", rootConfig.Host, rootConfig.Port)
	listener, err := rootConfig.NetworkListener()
	if err != nil {
		logger.Error("failed to listen on configured address", "address", address, "error", err)
		os.Exit(1)
	}

	url := fmt.Sprintf("https://%s/", address)
	logger.Info("Serving healthcheck endpoint", "url", url)

	if err := http.Serve(listener, router); err != nil {
		logger.Error("galera-agent http server exited", "error", err)
	}

	logger.Info("graceful-exit")
}
