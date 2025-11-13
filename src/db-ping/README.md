# Database Ping Utility

A simple utility that connects to a MySQL database and performs a ping operation with timeout support.

## Usage

Set the required environment variables and run the program:

```bash
export MYSQL_HOST="localhost"
export MYSQL_USERNAME="root"
export MYSQL_PASSWORD="password"
export TIMEOUT="10"  # timeout in seconds

go run main.go
```

## Environment Variables

- `MYSQL_HOST`: MySQL server hostname or IP address
- `MYSQL_USERNAME`: Database username
- `MYSQL_PASSWORD`: Database password  
- `TIMEOUT`: Connection timeout in seconds (must be positive integer)

## Exit Codes

- `0`: Success - database ping completed successfully
- `1`: Failure - connection failed, ping failed, timeout exceeded, or invalid parameters

## Features

- ✅ Reads database connectivity info from environment variables
- ✅ Configurable timeout parameter (in seconds)
- ✅ Always cleans up database connections via defer
- ✅ Performs connect+ping operations within specified timeout
- ✅ Proper error handling and exit codes
- ✅ Context-based timeout management
