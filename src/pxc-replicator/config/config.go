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
		Name    string `yaml:"name"`
		Host    string `yaml:"host"`
		Port    uint16 `yaml:"port"`
		Creds   Creds  `yaml:"creds"`
		Certs   Certs  `yaml:"certs"`
		Version string `yaml:"version"`
	}
	Creds struct {
		Username      string `yaml:"username"`
		Password      string `yaml:"password"`
		AdminUsername string `yaml:"admin_username"`
		AdminPassword string `yaml:"admin_password"`
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

func (t Target) AdminDSN() string {
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/", t.Creds.AdminUsername, t.Creds.AdminPassword, t.Host, t.Port)
}

// String returns a redacted MySQL DSN for this target
// It formats the credentials and host/port as "user:<redacted>@tcp(host:port)".
func (t Target) String() string {
	return fmt.Sprintf("%s:<redacted>@tcp(%s:%d)", t.Creds.Username, t.Host, t.Port)
}
