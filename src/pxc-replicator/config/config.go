// Package config holds the struct necessary to configure the connections and replication
package config

import (
	"fmt"
	"time"
)

type (
	Target struct {
		Name    string `json:"name" yaml:"name"`
		Host    string `json:"host" yaml:"host"`
		Port    uint16 `json:"port" yaml:"port"`
		Creds   Creds  `json:"creds" yaml:"creds"`
		Certs   Certs  `json:"certs" yaml:"certs"`
		TLS     Certs  `json:"tls" yaml:"tls"`
		Version string `json:"version" yaml:"version"`
	}
	HostConfig struct {
		Self              Target        `json:"self" yaml:"self"`
		Source            Target        `json:"source" yaml:"source"`
		DumpPath          string        `json:"dumpPath" yaml:"dumpPath" default:"/var/vcap/data/pxc-replicator/latest.sql"`
		WatchInterval     time.Duration `json:"watchInterval" yaml:"watchInterval" default:"5s"`
		ProcessTimeout    time.Duration `json:"gracefulTimeout" yaml:"gracefulTimeout" default:"300s"`
		ConnectionTimeout time.Duration
		MySQLBin          string `json:"mysqlBin" yaml:"mysqlBin"`
		DumpBin           string `json:"dumpBin" yaml:"dumpBin"`
	}
	Creds struct {
		Username, Password string
	}
	Certs struct {
		CA          string
		Certificate string
		PrivateKey  string
	}
)

// String creates a connection string with trailing slash
// e.g. `user:pass@tcp(host:port)/`
func (t Target) String() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/", t.Creds.Username, t.Creds.Password, t.Host, t.Port)
}
