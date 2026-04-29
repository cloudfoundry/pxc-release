package node_manager

import (
	"fmt"
	"net/http"
	"os"
	"sync"

	"code.cloudfoundry.org/lager/v3"
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 . MonitClient
type MonitClient interface {
	Start(serviceName string) error
	Stop(serviceName string) error
	Status(serviceName string) (string, error)
}

type NodeManager struct {
	ServiceName   string
	StateFilePath string
	MonitClient   MonitClient
	Logger        lager.Logger
	Mutex         *sync.Mutex
}

func (m *NodeManager) StartServiceBootstrap(_ *http.Request) (string, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	if m.ServiceName == "garbd" {
		return "", fmt.Errorf("bootstrapping arbitrator not allowed")
	}

	if err := os.WriteFile(m.StateFilePath, []byte("NEEDS_BOOTSTRAP"), 0777); err != nil {
		return "", fmt.Errorf("failed to initialize state file: %w", err)
	}

	if err := m.MonitClient.Start(m.ServiceName); err != nil {
		return "", err
	}

	return "cluster bootstrap successful", nil
}

func (m *NodeManager) StartServiceJoin(_ *http.Request) (string, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	if err := os.WriteFile(m.StateFilePath, []byte("CLUSTERED"), 0777); err != nil {
		return "", fmt.Errorf("failed to initialize state file: %w", err)
	}

	if err := m.MonitClient.Start(m.ServiceName); err != nil {
		return "", err
	}

	return "join cluster successful", nil
}

func (m *NodeManager) StartServiceSingleNode(_ *http.Request) (string, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	if err := os.WriteFile(m.StateFilePath, []byte("SINGLE_NODE"), 0777); err != nil {
		return "", fmt.Errorf("failed to initialize state file: %w", err)
	}

	if err := m.MonitClient.Start(m.ServiceName); err != nil {
		return "", err
	}

	return "single node start successful", nil
}

func (m *NodeManager) StopService(_ *http.Request) (string, error) {
	m.Mutex.Lock()
	defer m.Mutex.Unlock()

	if err := m.MonitClient.Stop(m.ServiceName); err != nil {
		return "", err
	}

	return "stop successful", nil
}

func (m *NodeManager) GetStatus(_ *http.Request) (string, error) {
	return m.MonitClient.Status(m.ServiceName)
}
