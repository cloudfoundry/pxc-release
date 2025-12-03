package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	_ "github.com/caarlos0/env/v11"
	"github.com/go-sql-driver/mysql"
)

func main() {
	var cfg Config
	if err := ParseConfig(&cfg); err != nil {
		fmt.Fprintf(os.Stderr, "Error: bad configuration - %s\n", err)
		os.Exit(1)
	}

	// Hard-coded/invalid "pingdb" username/pw are sufficient to exercise db connectivity.
	dsn := fmt.Sprintf("pingdb:pingdb@tcp(127.0.0.1:%s)/?tls=preferred", cfg.Port)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error opening database connection: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: Error closing database connection: %v\n", closeErr)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// Ping using timeout context, and expect a recognized MySQL-generated error in response.
	if err = db.PingContext(ctx); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// timed out
			_, _ = fmt.Fprintf(os.Stderr, "Error: database ping timeout after %s\n", cfg.Timeout)
			os.Exit(1)
		}

		// Recognized MySQL errors do not trigger failure (since the proxy contacted the db).
		if RecognizedMySQLError(err) {
			os.Exit(0)
		}

		_, _ = fmt.Fprintf(os.Stderr, "Error: database ping: %v\n", err.Error())
		os.Exit(2)
	}

	// Ping of user pingdb/pingdb succeeded.
	os.Exit(0)
}

// RecognizedMySQLError checks if the MySQL error is a server response error or some other error
func RecognizedMySQLError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}

	return mysqlErr.Number < 2000 || mysqlErr.Number >= 3000
}
