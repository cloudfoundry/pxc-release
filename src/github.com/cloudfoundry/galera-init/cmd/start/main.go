package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	_ "github.com/go-sql-driver/mysql"

	"github.com/cloudfoundry/galera-init/cluster_health_checker"
	"github.com/cloudfoundry/galera-init/config"
	"github.com/cloudfoundry/galera-init/db_helper"
	"github.com/cloudfoundry/galera-init/galera_init_status_server"
	"github.com/cloudfoundry/galera-init/os_helper"
	"github.com/cloudfoundry/galera-init/start_manager"
	"github.com/cloudfoundry/galera-init/start_manager/node_starter"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := config.NewConfig(os.Args)
	if err != nil {
		logger.Error("Error reading config", "error", err)
		os.Exit(1)
	}

	err = cfg.Validate()
	if err != nil {
		logger.Error("Error validating config", "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())

	setupSignals(cancel, logger)

	startManager, err := managerSetup(cfg, logger)
	if err != nil {
		logger.Info("Error initializing", "error", err)
		os.Exit(1)
	}

	logger.Info("starting")

	if err := startManager.Execute(ctx); err != nil {
		logger.Info("abnormal-termination", "error", err)
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
	logger.Info("exited")
}

func managerSetup(cfg *config.Config, logger *slog.Logger) (start_manager.StartManager, error) {
	OsHelper := os_helper.NewImpl()

	DBHelper := db_helper.NewDBHelper(
		OsHelper,
		&cfg.Db,
		cfg.LogFileLocation,
		logger,
	)

	ClusterHealthChecker := cluster_health_checker.NewClusterHealthChecker(
		cfg.ClusterUrls(),
		logger,
		cfg.HTTPClient(),
	)

	NodeStarter := node_starter.NewStarter(
		DBHelper,
		OsHelper,
		cfg.Manager,
		logger,
		ClusterHealthChecker,
	)

	listener, err := net.Listen("tcp", cfg.Manager.GaleraInitStatusServerAddress)
	if err != nil {
		return nil, err
	}

	galeraInitStatusServer := galera_init_status_server.NewGaleraInitStatusServer(listener)

	NodeStartManager := start_manager.New(
		OsHelper,
		cfg.Manager,
		DBHelper,
		NodeStarter,
		logger,
		ClusterHealthChecker,
		galeraInitStatusServer,
	)

	return NodeStartManager, nil
}

func setupSignals(shutdownMySQL func(), log *slog.Logger) {
	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, syscall.SIGTERM)

	go func() {
		for sig := range sigCh {
			log.Info("sigterm-received", "signal", sig)
			shutdownMySQL()
			log.Info("initiating-shutdown")
		}
	}()
}
