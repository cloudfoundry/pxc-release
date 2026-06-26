// Package config defines configuration structures for source/target nodes in pxc‑replicator.
package config

import (
	"fmt"
)

type (
	// Target represents a database endpoint that may act as source or target for replication.
	// The struct contains connection details, optional TLS certs and a MySQL version tag used for compatibility checks.
	//
	// Creds holds plain‑text authentication credentials required by the MySQL client library.
	//
	// Certs contains PEM encoded TLS materials. CA is the server certificate authority,
	// Certificate/PrivateKey form a client key pair used for encrypted connections.
	//
	// Config aggregates source and target Target structs along with local directories used for data
	// storage (DataDir) and binary location (BinDir).
	Target struct {
		Name    string `json:"name" yaml:"name"`
		Host    string `json:"host" yaml:"host"`
		Port    uint16 `json:"port" yaml:"port"`
		Creds   Creds  `json:"creds" yaml:"creds"`
		Certs   Certs  `json:"certs" yaml:"certs"`
		Version string `json:"version" yaml:"version"`
	}
	Creds struct {
		Username, Password string
	}
	Certs struct {
		CA          string `yaml:"ca"`
		Certificate string `yaml:"certificate"`
		PrivateKey  string `yaml:"private_key"`
	}
)

// DSN returns a MySQL DSN for this target, suitable to pass directly to sql.Open.
// It formats the credentials and host/port as "user:pass@tcp(host:port)/".
// The trailing slash indicates the root schema, which is required for replication commands.
func (t Target) DSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/", t.Creds.Username, t.Creds.Password, t.Host, t.Port)
}

// String returns a redacted MySQL DSN for this target
// It formats the credentials and host/port as "user:<redacted>@tcp(host:port)".
func (t Target) String() string {
	return fmt.Sprintf("%s:<redacted>@tcp(%s:%d)", t.Creds.Username, t.Host, t.Port)
}
