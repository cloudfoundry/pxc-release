package main

import (
	"context"
	"database/sql"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"code.cloudfoundry.org/lager/v3"
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

	ctx, cancel := context.WithCancel(context.Background())

	setupSignals(cancel, cfg.Logger)

	startManager, err := managerSetup(cfg)
	if err != nil {
		cfg.Logger.Info("manage-setup-failure", lager.Data{
			"error": err.Error(),
		})
		os.Exit(1)
	}

	cfg.Logger.Info("starting")

	if err := startManager.Execute(ctx); err != nil {
		cfg.Logger.Info("abnormal-termination", lager.Data{
			"error": err.Error(),
		})
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

func managerSetup(cfg *config.Config) (start_manager.StartManager, error) {
	OsHelper := os_helper.NewImpl()

	dsn := db_helper.FormatDSN(cfg.Db)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	DBHelper := db_helper.NewDBHelper(
		OsHelper,
		db,
		cfg.LogFileLocation,
		cfg.Logger,
	)

	ClusterHealthChecker := cluster_health_checker.NewClusterHealthChecker(
		cfg.ClusterUrls(),
		cfg.Logger,
		cfg.HTTPClient(),
	)

	NodeStarter := node_starter.NewStarter(
		DBHelper,
		OsHelper,
		cfg.Manager,
		cfg.Logger,
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
		cfg.Logger,
		ClusterHealthChecker,
		galeraInitStatusServer,
	)

	return NodeStartManager, nil
}

func setupSignals(shutdownMySQL func(), log lager.Logger) {
	sigCh := make(chan os.Signal, 1)

	signal.Notify(sigCh, syscall.SIGTERM)

	go func() {
		for sig := range sigCh {
			log.Info("sigterm-received", lager.Data{
				"signal": sig,
			})
			shutdownMySQL()
			log.Info("initiating-shutdown")
		}
	}()
}
