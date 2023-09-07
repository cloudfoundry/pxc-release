package domain

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
)

var BridgesProvider = NewBridges
var Dialer = net.Dial

type Backend struct {
	mutex          sync.RWMutex
	host           string
	port           uint
	statusPort     uint
	statusEndpoint string
	logger         *slog.Logger
	bridges        Bridges
	name           string
	healthy        bool
}

type BackendJSON struct {
	Host                string `json:"host"`
	Port                uint   `json:"port"`
	StatusPort          uint   `json:"status_port"`
	Healthy             bool   `json:"healthy"`
	Name                string `json:"name"`
	CurrentSessionCount uint   `json:"currentSessionCount"`
}

func NewBackend(
	name string,
	host string,
	port uint,
	statusPort uint,
	statusEndpoint string,
	logger *slog.Logger) *Backend {

	return &Backend{
		name:           name,
		host:           host,
		port:           port,
		statusPort:     statusPort,
		statusEndpoint: statusEndpoint,
		logger:         logger,
		bridges:        BridgesProvider(logger),
	}
}

func (b *Backend) HealthcheckUrls(useTLS bool) []string {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	if useTLS {
		return []string{fmt.Sprintf("https://%s:%d/%s", b.host, b.statusPort, b.statusEndpoint),
			fmt.Sprintf("http://%s:%d/%s", b.host, 9200, b.statusEndpoint)}
	}
	return []string{fmt.Sprintf("http://%s:%d/%s", b.host, b.statusPort, b.statusEndpoint)}
}

func (b *Backend) Bridge(clientConn net.Conn) error {
	backendAddr := fmt.Sprintf("%s:%d", b.host, b.port)

	backendConn, err := Dialer("tcp", backendAddr)
	if err != nil {
		return errors.New(fmt.Sprintf("Error establishing connection to backend: %s", err))
	}

	bridge := b.bridges.Create(clientConn, backendConn)
	bridge.Connect()
	_ = b.bridges.Remove(bridge) //untested

	return nil
}

func (b *Backend) SeverConnections() {
	b.logger.Info("Severing all connections to backend", "backend", b)
	b.bridges.RemoveAndCloseAll()
}

func (b *Backend) SetHealthy() {
	if !b.Healthy() {
		b.logger.Info("Previously unhealthy backend became healthy", "backend", b)
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.healthy = true
}

func (b *Backend) SetUnhealthy() {
	if b.Healthy() {
		b.logger.Info("Previously healthy backend became unhealthy.", "backend", b)
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()
	b.healthy = false
}

func (b *Backend) Healthy() bool {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	return b.healthy
}

func (b *Backend) AsJSON() BackendJSON {
	b.mutex.RLock()
	defer b.mutex.RUnlock()
	return BackendJSON{
		Host:                b.host,
		Port:                b.port,
		StatusPort:          b.statusPort,
		Name:                b.name,
		Healthy:             b.healthy,
		CurrentSessionCount: b.bridges.Size(),
	}
}

func (b *Backend) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("name", b.name),
		slog.Bool("healthy", b.healthy),
		slog.String("host", b.host),
		slog.Uint64("port", uint64(b.port)),
		slog.Uint64("status_port", uint64(b.statusPort)),
		slog.Uint64("currentSessionCount", uint64(b.bridges.Size())),
	)
}
