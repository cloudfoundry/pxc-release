package main

import (
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"

	"code.cloudfoundry.org/lager/v3"

	"github.com/tedsuo/ifrit"
	"github.com/tedsuo/ifrit/grouper"
	"github.com/tedsuo/ifrit/sigmon"

	"github.com/cloudfoundry-incubator/switchboard/api"
	"github.com/cloudfoundry-incubator/switchboard/apiaggregator"
	"github.com/cloudfoundry-incubator/switchboard/config"
	"github.com/cloudfoundry-incubator/switchboard/domain"
	"github.com/cloudfoundry-incubator/switchboard/metrics"
	"github.com/cloudfoundry-incubator/switchboard/runner/bridge"
	httprunner "github.com/cloudfoundry-incubator/switchboard/runner/http"
	"github.com/cloudfoundry-incubator/switchboard/runner/monitor"
)

func main() {
	rootConfig, err := config.NewConfig(os.Args)

	logger := rootConfig.Logger

	err = rootConfig.Validate()
	if err != nil {
		logger.Fatal("Error validating config:", err, lager.Data{"config": rootConfig})
	}

	if _, err := os.Stat(rootConfig.StaticDir); os.IsNotExist(err) {
		logger.Fatal(fmt.Sprintf("staticDir: %s does not exist", rootConfig.StaticDir), nil)
	}

	serverTLSConfig, err := rootConfig.ServerTLSConfig()
	if err != nil {
		logger.Fatal("load-tls-config", err)
	}

	backends := domain.NewBackends(rootConfig.Proxy.Backends, logger)

	client := rootConfig.HTTPClient()

	activeNodeClusterMonitor := monitor.NewClusterMonitor(client, rootConfig.GaleraAgentTLS.Enabled, backends, rootConfig.Proxy.HealthcheckTimeout(), logger.Session("active-monitor"), true)

	activeNodeBridgeRunner := bridge.NewRunner(
		fmt.Sprintf("%s:%d", rootConfig.BindAddress, rootConfig.Proxy.Port),
		rootConfig.Proxy.ShutdownDelay(),
		logger.Session("active-bridge-runner"),
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

	if rootConfig.Metrics.Enabled {
		metricsEmitter := metrics.New(backends)
		members = append(members, grouper.Member{
			Name:   "metrics",
			Runner: httprunner.NewRunner(fmt.Sprintf("localhost:%d", rootConfig.Metrics.Port), metricsEmitter.Handler(), serverTLSConfig, rootConfig.API.TLS.Enabled),
		})
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
		inactiveNodeClusterMonitor := monitor.NewClusterMonitor(client, rootConfig.GaleraAgentTLS.Enabled, backends, rootConfig.Proxy.HealthcheckTimeout(), logger.Session("inactive-monitor"), false)

		inactiveNodeBridgeRunner := bridge.NewRunner(
			fmt.Sprintf("%s:%d", rootConfig.BindAddress, rootConfig.Proxy.InactiveMysqlPort),
			0,
			logger.Session("inactive-bridge-runner"),
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

	logger.Info("Proxy started", lager.Data{"proxyConfig": rootConfig.Proxy})

	err = <-process.Wait()
	if err != nil {
		logger.Fatal("Switchboard exited unexpectedly", err, lager.Data{"proxyConfig": rootConfig.Proxy})
	}
}
