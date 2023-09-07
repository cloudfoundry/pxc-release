package main

import (
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"

	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/sigmon"

	"github.com/cloudfoundry-incubator/switchboard/api"
	"github.com/cloudfoundry-incubator/switchboard/apiaggregator"
	"github.com/cloudfoundry-incubator/switchboard/config"
	"github.com/cloudfoundry-incubator/switchboard/domain"
	"github.com/cloudfoundry-incubator/switchboard/runner/bridge"
	httprunner "github.com/cloudfoundry-incubator/switchboard/runner/http"
	"github.com/cloudfoundry-incubator/switchboard/runner/monitor"
)

func main() {
	rootConfig, err := config.NewConfig(os.Args)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: rootConfig.LogLevel}))
	if err != nil {
		logger.Error("Error reading config", "error", err, "config", rootConfig)
		os.Exit(1)
	}

	err = rootConfig.Validate()
	if err != nil {
		logger.Error("Error validating config", "error", err, "config", rootConfig)
		os.Exit(1)
	}

	if _, err := os.Stat(rootConfig.StaticDir); os.IsNotExist(err) {
		logger.Error("dashboard static directory does not exist", "path", rootConfig.StaticDir)
	}

	serverTLSConfig, err := rootConfig.ServerTLSConfig()
	if err != nil {
		logger.Error("Initializing TLS config failed", "error", err)
		os.Exit(1)
	}

	backends := domain.NewBackends(rootConfig.Proxy.Backends, logger)

	client := rootConfig.HTTPClient()

	activeNodeClusterMonitor := monitor.NewClusterMonitor(client, rootConfig.GaleraAgentTLS.Enabled, backends, rootConfig.Proxy.HealthcheckTimeout(), logger.With("component", "active-monitor"), true)

	activeNodeBridgeRunner := bridge.NewRunner(
		fmt.Sprintf("%s:%d", rootConfig.BindAddress, rootConfig.Proxy.Port),
		rootConfig.Proxy.ShutdownDelay(),
		logger.With("task", "active-bridge-runner"),
	)
	clusterStateManager := api.NewClusterAPI(logger)

	activeNodeClusterMonitor.RegisterBackendSubscriber(activeNodeBridgeRunner.ActiveBackendChan)
	activeNodeClusterMonitor.RegisterBackendSubscriber(clusterStateManager.ActiveBackendChan)

	clusterStateManager.RegisterTrafficEnabledChan(activeNodeBridgeRunner.TrafficEnabledChan)
	go clusterStateManager.ListenForActiveBackend()

	apiHandler := api.NewHandler(clusterStateManager, backends, logger, rootConfig.API, rootConfig.StaticDir)
	aggregatorHandler := apiaggregator.NewHandler(logger, rootConfig.API)

	members := grouper.Members{
		{
			Name:   "active-node-bridge",
			Runner: activeNodeBridgeRunner,
		},
		{
			Name: "api-aggregator",
			Runner: httprunner.NewRunner(
				fmt.Sprintf("%s:%d", rootConfig.BindAddress, rootConfig.API.AggregatorPort),
				aggregatorHandler,
				serverTLSConfig,
				rootConfig.API.TLS.Enabled,
			),
		},
		{
			Name: "api",
			Runner: httprunner.NewRunner(
				fmt.Sprintf("%s:%d", rootConfig.BindAddress, rootConfig.API.Port),
				apiHandler,
				serverTLSConfig,
				rootConfig.API.TLS.Enabled,
			),
		},
		{
			Name:   "active-node-monitor",
			Runner: monitor.NewRunner(activeNodeClusterMonitor, logger),
		},
	}

	if rootConfig.HealthPort != rootConfig.API.Port {
		members = append(members, grouper.Member{
			Name: "health",
			Runner: httprunner.NewRunner(
				fmt.Sprintf("%s:%d", rootConfig.BindAddress, rootConfig.HealthPort),
				http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {}),
				serverTLSConfig,
				rootConfig.API.TLS.Enabled,
			),
		})
	}

	if rootConfig.Proxy.InactiveMysqlPort != 0 {
		inactiveNodeClusterMonitor := monitor.NewClusterMonitor(client, rootConfig.GaleraAgentTLS.Enabled, backends, rootConfig.Proxy.HealthcheckTimeout(), logger.With("component", "inactive-monitor"), false)

		inactiveNodeBridgeRunner := bridge.NewRunner(
			fmt.Sprintf("%s:%d", rootConfig.BindAddress, rootConfig.Proxy.InactiveMysqlPort),
			0,
			logger.With("component", "inactive-bridger-runner"),
		)

		inactiveNodeClusterMonitor.RegisterBackendSubscriber(inactiveNodeBridgeRunner.ActiveBackendChan)
		clusterStateManager.RegisterTrafficEnabledChan(inactiveNodeBridgeRunner.TrafficEnabledChan)

		members = append(members,
			grouper.Member{
				Name:   "inactive-node-bridge",
				Runner: inactiveNodeBridgeRunner,
			},
			grouper.Member{
				Name:   "inactive-node-monitor",
				Runner: monitor.NewRunner(inactiveNodeClusterMonitor, logger),
			},
		)
	}

	group := grouper.NewOrdered(os.Interrupt, members)
	process := ifrit.Invoke(sigmon.New(group))

	logger.Info("Proxy started", "proxyConfig", rootConfig.Proxy)

	err = <-process.Wait()
	if err != nil {
		logger.Error("Switchboard exited unexpectedly", "error", err, "proxyConfig", rootConfig.Proxy)
		os.Exit(1)
	}
}
