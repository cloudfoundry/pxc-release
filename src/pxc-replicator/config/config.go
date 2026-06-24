// Package config holds the struct necessary to configure the connections and replication
package config

import (
	"fmt"
)

type (
	Target struct {
		Name    string `json:"name" yaml:"name"`
		Host    string `json:"host" yaml:"host"`
		Port    uint16 `json:"port" yaml:"port"`
		Creds   Creds  `json:"creds" yaml:"creds"`
		Certs   *Certs `json:"certs" yaml:"certs"`
		Version string `json:"version" yaml:"version"`
	}
	Creds struct {
		Username, Password string
	}
	Certs struct {
		CA          []byte
		Certificate []byte
		PrivateKey  []byte
	}
	Config struct {
		Source  Target `yaml:"source"`
		Target  Target `yaml:"target"`
		BinDir  string
		DataDir string
	}
)

// String creates a connection string with trailing slash
// e.g. `user:pass@tcp(host:port)/`
func (t Target) String() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/", t.Creds.Username, t.Creds.Password, t.Host, t.Port)
}
