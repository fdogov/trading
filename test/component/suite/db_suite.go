package suite

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/fdogov/trading/internal/config"
	"github.com/fdogov/trading/internal/store"
	"github.com/fdogov/trading/internal/store/postgres"

	// Import PostgreSQL driver for database connection
	_ "github.com/lib/pq"
)

// DBSuite provides functionality for database tests
type DBSuite struct {
	BaseSuite

	// DB connections
	DB         *sqlx.DB
	DBHandler  *postgres.DB
	testSchema string

	// Configuration
	Config *config.Config

	// Transactor for services using the test transaction
	Transactor *postgres.Transactor

	// Stores
	AccountStore store.AccountStore
}

// SetupSuite initializes the test environment with a database
func (s *DBSuite) SetupSuite() {
	// Call the parent method to set up common components
	s.BaseSuite.SetupSuite()

	var err error

	// Load configuration from config_test.yaml
	s.loadConfig()

	// Check if the test database exists and create it if necessary
	s.ensureTestDatabaseExists()

	// Connect to the existing database
	s.DB, err = s.connectToTradingDB()
	s.Require().NoError(err)

	// Create DBHandler for use in tests
	s.DBHandler, err = postgres.NewDB(s.Config.Database)
	s.Require().NoError(err)

	// Create Transactor for transactions
	s.Transactor = postgres.NewTransactor(s.DBHandler)

	// Create a unique schema for tests
	testPrefix := fmt.Sprintf("test_%s_%s",
		s.T().Name()[:minInt(10, len(s.T().Name()))], // Prefix from test name
		uuid.New().String()[:8])                      // Unique suffix
	s.testSchema = testPrefix
	err = s.createTestSchema(testPrefix)
	s.Require().NoError(err)

	// Apply migrations
	s.applyMigrations()

	// Initialize repositories
	s.initRepositories()
}

// SetupTest prepares the test environment before each test
func (s *DBSuite) SetupTest() {
	// Clean tables before each test
	s.cleanupTables()
}

// TearDownTest cleans resources after each test
func (s *DBSuite) TearDownTest() {
	// Additional cleanup
}

// TearDownSuite releases resources after tests are completed
func (s *DBSuite) TearDownSuite() {
	if s.DB != nil && s.testSchema != "" {
		// Delete test schema with all tables
		s.T().Logf("Cleaning up test schema %s", s.testSchema)
		_, err := s.DB.Exec(fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", s.testSchema))
		if err != nil {
			s.T().Logf("Failed to delete schema: %v", err)
		}

		// Close connections
		s.DB.Close()
		if s.DBHandler != nil {
			s.DBHandler.Close()
		}
	}

	// Delete the test database if it was created by us
	dbName := s.Config.Database.DBName
	if dbName != "" && strings.Contains(dbName, "_") { // Check that this is actually a test database
		dbHost := s.Config.Database.Host
		dbPort := s.Config.Database.Port
		dbUser := s.Config.Database.User
		dbPass := s.Config.Database.Password

		// Connect to postgres to delete the database
		connStr := fmt.Sprintf(
			"host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
			dbHost, dbPort, dbUser, dbPass,
		)

		db, err := sql.Open("postgres", connStr)
		if err != nil {
			s.T().Logf("Failed to connect to postgres to delete database: %v", err)
			return
		}
		defer db.Close()

		// Check that the database exists before deleting
		var exists bool
		err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists)

		if err != nil {
			s.T().Logf("Failed to check database existence: %v", err)
		} else if exists {
			// Close all connections to the database
			_, err = db.Exec(fmt.Sprintf("SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s'", dbName))
			if err != nil {
				s.T().Logf("Failed to close database connections: %v", err)
			}

			// Delete the database
			_, err = db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", dbName))
			if err != nil {
				s.T().Logf("Failed to delete database %s: %v", dbName, err)
			} else {
				s.T().Logf("Database %s successfully deleted", dbName)
			}
		}
	}
}

// loadConfig loads configuration from config_test.yaml
func (s *DBSuite) loadConfig() {
	// Set CONFIG_PATH environment variable if it's not set
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		// Determine absolute path to config_test.yaml
		projectRoot, err := filepath.Abs("../../../")
		if err != nil {
			s.T().Logf("Failed to determine project root: %v", err)
			s.T().Fatalf("Configuration initialization error")
		}

		configTestPath := filepath.Join(projectRoot, "config", "config_test.yaml")
		if _, err := os.Stat(configTestPath); os.IsNotExist(err) {
			s.T().Logf("Configuration file not found at path: %s", configTestPath)
			s.T().Fatalf("Configuration initialization error")
		}

		s.T().Logf("Using configuration from: %s", configTestPath)
		err = os.Setenv("CONFIG_PATH", configTestPath)
		if err != nil {
			s.T().Logf("Failed to set CONFIG_PATH: %v", err)
			s.T().Fatalf("Configuration initialization error")
		}
	}

	// Load configuration
	cfg := config.LoadConfig()

	s.Config = &cfg
	s.T().Logf("Configuration successfully loaded. Database settings: %s@%s:%s/%s",
		s.Config.Database.User, s.Config.Database.Host, s.Config.Database.Port, s.Config.Database.DBName)
}

// ensureTestDatabaseExists checks if the test database exists and creates it if necessary
func (s *DBSuite) ensureTestDatabaseExists() {
	dbHost := s.Config.Database.Host
	dbPort := s.Config.Database.Port
	dbUser := s.Config.Database.User
	dbPass := s.Config.Database.Password
	dbName := s.Config.Database.DBName

	// Connection string to the main database postgres
	connStr := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		dbHost, dbPort, dbUser, dbPass,
	)

	s.T().Logf("Checking if database %s exists...", dbName)
	s.T().Logf("Using connection settings: host=%s, port=%s, user=%s", dbHost, dbPort, dbUser)

	// Connect to postgres database
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		s.T().Logf("Failed to connect to postgres: %v", err)
		s.T().Logf("Check settings in config_test.yaml")
		return
	}
	defer db.Close()

	// Check connection to the database
	if err := db.Ping(); err != nil {
		s.T().Logf("Failed to establish connection to postgres: %v", err)
		s.T().Logf("Check settings in config_test.yaml or ensure PostgreSQL server is running")
		return
	}

	// Check if database exists
	var exists bool
	err = db.QueryRow("SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname = $1)", dbName).Scan(&exists)
	if err != nil {
		s.T().Logf("Failed to check if database exists: %v", err)
		return
	}

	// If database doesn't exist, create it
	if !exists {
		s.T().Logf("Database %s doesn't exist, creating...", dbName)

		// In PostgreSQL, you can't use parameterized queries for database names,
		// so we create an SQL string directly, first checking the database name
		// for valid characters to prevent SQL injection
		if !isValidDBName(dbName) {
			s.T().Fatalf("Invalid database name: %s", dbName)
			return
		}

		_, err = db.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName))
		if err != nil {
			s.T().Logf("Failed to create database: %v", err)
			return
		}

		s.T().Logf("Database %s successfully created", dbName)
	} else {
		s.T().Logf("Database %s already exists", dbName)
	}
}

// isValidDBName checks that the database name contains only valid characters
func isValidDBName(name string) bool {
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
			return false
		}
	}
	return true
}

// connectToTradingDB connects to the trading database
func (s *DBSuite) connectToTradingDB() (*sqlx.DB, error) {
	dbHost := s.Config.Database.Host
	dbPort := s.Config.Database.Port
	dbUser := s.Config.Database.User
	dbPass := s.Config.Database.Password
	dbName := s.Config.Database.DBName

	s.T().Logf("Connecting to database %s on %s:%s with user %s", dbName, dbHost, dbPort, dbUser)

	// Create connection string
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		dbHost, dbPort, dbUser, dbPass, dbName, s.Config.Database.SSLMode,
	)

	var db *sqlx.DB
	var err error
	for attempts := 0; attempts < 5; attempts++ {
		db, err = sqlx.Connect("postgres", dsn)
		if err == nil {
			pingErr := db.Ping()
			if pingErr == nil {
				s.T().Logf("Successfully connected to database %s", dbName)
				return db, nil
			}
			db.Close()
			err = pingErr
		}
		s.T().Logf("Failed to connect to database %s attempt %d: %v, retrying...", dbName, attempts+1, err)
		time.Sleep(time.Duration(attempts+1) * time.Second)
	}

	return nil, fmt.Errorf("failed to connect to database %s: %w", dbName, err)
}

// createTestSchema creates a unique schema for tests
func (s *DBSuite) createTestSchema(schemaName string) error {
	s.T().Logf("Creating test schema %s", schemaName)

	// Create schema
	_, err := s.DB.Exec(fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s", schemaName))
	if err != nil {
		return fmt.Errorf("failed to create schema %s: %w", schemaName, err)
	}

	// Set schema as current for the session
	_, err = s.DB.Exec(fmt.Sprintf("SET search_path TO %s, public", schemaName))
	if err != nil {
		return fmt.Errorf("failed to set search_path for schema %s: %w", schemaName, err)
	}

	s.T().Logf("Successfully created and set schema %s", schemaName)
	return nil
}

// createMigrationScript creates a migration script if it doesn't exist
func (s *DBSuite) createMigrationScript() {
	// Determine relative path to migrations directory
	projectRoot, err := filepath.Abs("../../../")
	if err != nil {
		s.T().Logf("Failed to determine project root: %v", err)
		return
	}

	scriptDir := filepath.Join(projectRoot, "scripts")
	scriptPath := filepath.Join(scriptDir, "migrate.sh")

	// If scripts directory doesn't exist, create it
	if _, err := os.Stat(scriptDir); os.IsNotExist(err) {
		if err := os.MkdirAll(scriptDir, 0755); err != nil {
			s.T().Logf("Failed to create scripts directory: %v", err)
			return
		}
	}

	// If migration script doesn't exist, create it
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		scriptContent := `#!/bin/bash

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
`
		if err := os.WriteFile(scriptPath, []byte(scriptContent), 0755); err != nil {
			s.T().Logf("Failed to create migration script: %v", err)
			return
		}

		s.T().Logf("Migration script %s successfully created", scriptPath)
	}
}

// applyMigrations applies migrations to the test database
func (s *DBSuite) applyMigrations() {
	// Check for and create migration script if necessary
	s.createMigrationScript()

	// Determine relative path to migrations directory
	projectRoot, err := filepath.Abs("../../../")
	if err != nil {
		s.T().Logf("Failed to determine project root: %v", err)
		return
	}

	migrationsDir := filepath.Join(projectRoot, "migrations")

	// Check if migrations directory exists
	if _, err := os.Stat(migrationsDir); os.IsNotExist(err) {
		s.T().Fatalf("Migrations directory not found at %s: %v", migrationsDir, err)
		return
	}

	s.T().Logf("Using migrations from: %s", migrationsDir)

	// Path to sql-migrate configuration file
	dbconfigPath := filepath.Join(projectRoot, "dbconfig.yml")
	if _, err := os.Stat(dbconfigPath); os.IsNotExist(err) {
		s.T().Fatalf("File dbconfig.yml not found at %s", dbconfigPath)
		return
	}

	// Create temporary config for sql-migrate with our test schema
	tmpConfigDir, err := os.MkdirTemp("", "sql-migrate-config")
	if err != nil {
		s.T().Fatalf("Error creating temporary directory for config: %v", err)
		return
	}
	defer os.RemoveAll(tmpConfigDir)

	// Create temporary configuration file for sql-migrate
	configPath := filepath.Join(tmpConfigDir, "dbconfig.yml")
	configContent := fmt.Sprintf(`
development:
  dialect: postgres
  datasource: host=%s port=%s dbname=%s user=%s password=%s sslmode=%s search_path=%s
  dir: %s
  table: migrations
`,
		s.Config.Database.Host,
		s.Config.Database.Port,
		s.Config.Database.DBName,
		s.Config.Database.User,
		s.Config.Database.Password,
		s.Config.Database.SSLMode,
		s.testSchema,
		migrationsDir)

	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		s.T().Fatalf("Error creating configuration file: %v", err)
		return
	}

	// Path to migration script
	scriptPath := filepath.Join(projectRoot, "scripts", "migrate.sh")

	// Print command for debugging
	cmdStr := fmt.Sprintf("%s up -config %s -env development", scriptPath, configPath)
	s.T().Logf("Running migration command: %s", cmdStr)

	// Execute migration command
	cmd := exec.Command(scriptPath, "up", "-config", configPath, "-env", "development")
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.T().Fatalf("Error executing migrations: %v\nOutput: %s", err, string(output))
		return
	}

	s.T().Logf("Migrations successfully applied: %s", string(output))
}

// initRepositories initializes repositories with database connection
func (s *DBSuite) initRepositories() {
	// Initialize repositories with real database
	// Here real repositories are connected, not mocks
	s.AccountStore = postgres.NewAccountStore(s.DBHandler)
}

// cleanupTables cleans tables before each test
func (s *DBSuite) cleanupTables() {
	// Clean tables
	tables := []string{"accounts"}
	for _, table := range tables {
		_, err := s.DB.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table))
		if err != nil {
			s.T().Logf("Error cleaning table %s: %v", table, err)
		}
	}
}

// getEnvOrDefault returns environment variable value or default value
func getEnvOrDefault(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// minInt returns minimum of two integers
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
