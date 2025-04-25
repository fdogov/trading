#!/bin/bash
# Script for running migrations inside Docker container

set -e

POSTGRES_HOST=${DB_HOST:-postgres}
POSTGRES_PORT=${DB_PORT:-5432}
POSTGRES_USER=${DB_USER:-postgres}
POSTGRES_PASSWORD=${DB_PASSWORD:-postgres}
POSTGRES_DB=${DB_NAME:-trading}

# Check PostgreSQL availability
until pg_isready -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER; do
  >&2 echo "PostgreSQL is not available - waiting"
  sleep 1
done

# Try to create database (ignore error if DB already exists)
PGPASSWORD=$POSTGRES_PASSWORD psql -h $POSTGRES_HOST -p $POSTGRES_PORT -U $POSTGRES_USER -c "CREATE DATABASE $POSTGRES_DB;" || true

# Run migrations
echo "Running migrations..."
migrate -path=/app/migrations -database "postgres://$POSTGRES_USER:$POSTGRES_PASSWORD@$POSTGRES_HOST:$POSTGRES_PORT/$POSTGRES_DB?sslmode=disable" up

echo "Migrations completed successfully"
