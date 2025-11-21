package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		fmt.Fprintf(os.Stderr, "Error: PORT environment variable is required\n")
		os.Exit(1)
	}

	timeoutStr := os.Getenv("TIMEOUT")
	if timeoutStr == "" {
		fmt.Fprintf(os.Stderr, "Error: TIMEOUT environment variable is required (in seconds)\n")
		os.Exit(1)
	}

	timeout, err := strconv.Atoi(timeoutStr)
	if err != nil || timeout <= 0 {
		fmt.Fprintf(os.Stderr, "Error: TIMEOUT must be a positive integer (in seconds)\n")
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

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	// Ping using timeout context, and expect a recognized MySQL-generated error in response.
	if err := db.PingContext(ctx); err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			// timed out
			_, _ = fmt.Fprintf(os.Stderr, "Error: database ping timeout after %d seconds\n", timeout)
			os.Exit(1)
		}

		// Recognized MySQL errors do not trigger failure (since the proxy contacted the db).
		if RecognizedMySQLError(err.Error()) {
			os.Exit(0)
		}

		_, _ = fmt.Fprintf(os.Stderr, "Error: database ping: %v\n", err.Error())
		os.Exit(2)
	}

	// Ping of user pingdb/pingdb succeeded.
	os.Exit(0)
}

// RecognizedMySQLError checks if a string starts with "Error NNNN (XXXXX):" pattern
// where NNNN is one or more digits and XXXXX is one or more alphanumeric characters
func RecognizedMySQLError(input string) bool {
	// Regex pattern breakdown:
	// ^Error\s+     - starts with "Error" followed by one or more whitespace
	// \d+           - one or more digits
	// \s+           - one or more whitespace
	// \(            - literal opening parenthesis
	// [a-zA-Z0-9]+  - one or more alphanumeric characters (letters or digits)
	// \)            - literal closing parenthesis
	// :             - literal colon

	pattern := `^Error\s+\d+\s+\([a-zA-Z0-9]+\):`

	matched, err := regexp.MatchString(pattern, input)
	if err != nil {
		return false
	}

	return matched
}
