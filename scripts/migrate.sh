#!/bin/bash

set -e

# Define the path to GOPATH and GOBIN
GOPATH=${GOPATH:-$(go env GOPATH)}
GOBIN=${GOBIN:-"$GOPATH/bin"}

# Add GOBIN to PATH
export PATH="$GOBIN:$PATH"

# Check if sql-migrate is installed
if ! command -v sql-migrate &> /dev/null; then
    echo "sql-migrate is not installed, installing..."
    go install github.com/rubenv/sql-migrate/...@latest
    
    # Explicitly specify the path to the executable if it's not in PATH
    SQL_MIGRATE="$GOBIN/sql-migrate"
else
    SQL_MIGRATE="sql-migrate"
fi

# Run sql-migrate with the passed arguments
echo "Running migrations using $SQL_MIGRATE"
"$SQL_MIGRATE" "$@"
