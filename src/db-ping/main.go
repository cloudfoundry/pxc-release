package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	// Read environment variables
	mysqlHost := os.Getenv("MYSQL_HOST")
	if mysqlHost == "" {
		fmt.Fprintf(os.Stderr, "Error: MYSQL_HOST environment variable is required\n")
		os.Exit(1)
	}

	mysqlUser := os.Getenv("MYSQL_USERNAME")
	if mysqlUser == "" {
		fmt.Fprintf(os.Stderr, "Error: MYSQL_USERNAME environment variable is required\n")
		os.Exit(1)
	}

	mysqlPassword := os.Getenv("MYSQL_PASSWORD")
	if mysqlPassword == "" {
		fmt.Fprintf(os.Stderr, "Error: MYSQL_PASSWORD environment variable is required\n")
		os.Exit(1)
	}

	timeoutStr := os.Getenv("TIMEOUT")
	if timeoutStr == "" {
		fmt.Fprintf(os.Stderr, "Error: TIMEOUT environment variable is required (in seconds)\n")
		os.Exit(1)
	}

	timeoutSeconds, err := strconv.Atoi(timeoutStr)
	if err != nil || timeoutSeconds <= 0 {
		fmt.Fprintf(os.Stderr, "Error: TIMEOUT must be a positive integer (seconds)\n")
		os.Exit(1)
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// Build DSN (Data Source Name)
	dsn := fmt.Sprintf("%s:%s@tcp(%s:3306)/?tls=preferred", mysqlUser, mysqlPassword, mysqlHost)

	// Perform database operations within timeout
	exitCode := performDatabasePing(ctx, dsn)
	os.Exit(exitCode)
}

func performDatabasePing(ctx context.Context, dsn string) int {
	// Open database connection
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "Error opening database connection: %v\n", err)
		return 1
	}

	// Ensure database connection is always closed (cleanup)
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			_, _ = fmt.Fprintf(os.Stderr, "Warning: Error closing database connection: %v\n", closeErr)
		}
	}()

	// Perform ping operation with timeout context
	if err := db.PingContext(ctx); err != nil {
		// Check if it was a timeout or other error
		if ctx.Err() == context.DeadlineExceeded {
			_, _ = fmt.Fprintf(os.Stderr, "Error: Database ping timed out\n")
		} else {
			_, _ = fmt.Fprintf(os.Stderr, "Error pinging database: %v\n", err)
		}
		return 1
	}

	fmt.Println("Database ping successful")
	return 0
}
