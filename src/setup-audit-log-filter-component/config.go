package main

import (
	"database/sql"
	"fmt"

	"github.com/caarlos0/env/v11"
	"github.com/go-sql-driver/mysql"
)

type MySQLDB struct {
	*sql.DB
	cfg *mysql.Config
}

type Config struct {
	MySQL        MySQLDB  `env:"MYSQL_DSN"`
	ExcludeUsers []string `env:"MYSQL_AUDIT_EXCLUDE_USERS"`
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
		return fmt.Errorf("error initialize mysql connector: %s", err)
	}

	m.cfg = mysqlCfg
	m.DB = sql.OpenDB(mysqlConnector)

	return nil
}

func ParseConfig(cfg *Config) error {
	err := env.Parse(cfg)
	return err
}
