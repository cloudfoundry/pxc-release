package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		fmt.Fprintf(os.Stderr, "Error: PORT environment variable is required\n")
		os.Exit(1)
	}

	timeoutStr := os.Getenv("TIMEOUT")
	if timeoutStr == "" {
		fmt.Fprintf(os.Stderr, "Error: TIMEOUT environment variable is required\n")
		os.Exit(1)
	}

	timeout, err := time.ParseDuration(timeoutStr)
	if err != nil || timeout <= 0 {
		fmt.Fprintf(os.Stderr, "Error: TIMEOUT must be a go parsable interval\n")
		os.Exit(1)
	}

	// Hard-coded/invalid "pingdb" username/pw are sufficient to exercise db connectivity.
	dsn := fmt.Sprintf("pingdb:pingdb@tcp(127.0.0.1:%s)/?tls=preferred", port)
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

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Ping using timeout context, and expect a recognized MySQL-generated error in response.
	if err := db.PingContext(ctx); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// timed out
			_, _ = fmt.Fprintf(os.Stderr, "Error: database ping timeout after %.0f seconds\n", timeout.Seconds())
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

func RecognizedMySQLError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}

	// Test for MySQL Server errors (proof MySQL responded via the proxy)
	// Server error ranges: 1000-1999, 3000-9999
	// Client error range: 2000-2999 (connection issues, other local client errors)
	//
	// Common expected server errors:
	// - 1045: Access denied (our invalid pingdb/pingdb credentials)
	// - 1129: Host blocked
	// - 1130: Host not allowed to connect
	//
	// Client errors we DON'T want to accept:
	// - 2002: Can't connect (network issue)
	// - 2003: Can't connect to server (connection refused)
	// - 2005: Unknown host

	return mysqlErr.Number < 2000 || mysqlErr.Number >= 3000
}
