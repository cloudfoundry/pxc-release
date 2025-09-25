package main

import (
	"database/sql"
	"fmt"

	"github.com/caarlos0/env/v11"
	"github.com/go-sql-driver/mysql"
)

type MySQLDB struct {
	*sql.DB
	Cfg *mysql.Config
}

type Config struct {
	MySQL         MySQLDB  `env:"MYSQL_DSN,required"`
	ExcludeUsers  []string `env:"MYSQL_AUDIT_EXCLUDE_USERS"`
	DefaultFilter string   `env:"MYSQL_AUDIT_LOG_FILTER,required"`
}

func (m *MySQLDB) UnmarshalText(data []byte) error {
	dsn := string(data)
	mysqlCfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("invalid MySQL DSN: %s", err)
	}

	mysqlCfg.InterpolateParams = true

	mysqlConnector, err := mysql.NewConnector(mysqlCfg)
	if err != nil {
		// Note: Untested; This can only fail if ParseDSN also failed
		//       Error path handled here defensively.
		return fmt.Errorf("error initializing mysql connector: %s", err)
	}

	m.Cfg = mysqlCfg
	m.DB = sql.OpenDB(mysqlConnector)

	return nil
}

func ParseConfig(cfg *Config) error {
	if err := env.Parse(cfg); err != nil {
		return err
	}

	return nil
}
