package config

import (
	"os"
)

// Config represents the application configuration
type Config struct {
	GRPC         GRPC               `yaml:"grpc"`
	Database     Database           `yaml:"database"`
	Kafka        Kafka              `yaml:"kafka"`
	Dependencies DependenciesConfig `yaml:"dependencies"`
	Logger       Logger             `yaml:"logger"`
}

// GRPC represents the gRPC server configuration
type GRPC struct {
	Port string `yaml:"port"`
}

// Database represents the database configuration
type Database struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	DBName   string `yaml:"dbname"`
	SSLMode  string `yaml:"sslmode"`
}

// Kafka represents the Kafka configuration
type Kafka struct {
	Brokers           []string `yaml:"brokers"`
	GroupID           string   `yaml:"group_id"`
	AccountTopic      string   `yaml:"account_topic"`
	DepositEventTopic string   `yaml:"deposit_event_topic"`
	DepositFeedTopic  string   `yaml:"deposit_feed_topic"`
	OrderEventTopic   string   `yaml:"order_event_topic"`
}

// Logger represents the logger configuration
type Logger struct {
	Level      string `yaml:"level"`
	Encoding   string `yaml:"encoding"`
	OutputPath string `yaml:"output_path"`
}

// Dependency represents the external dependencies configuration
type Dependency struct {
	Address string `yaml:"address"`
	Timeout string `yaml:"timeout"`
	Retries uint   `yaml:"retries"`
}
type DependenciesConfig struct {
	PartnerProxy Dependency `yaml:"partnerProxy"`
}

// LoadConfig loads configuration from environment variables or config file
func LoadConfig() Config {
	// TODO: Implement loading configuration from file or environment variables
	return Config{
		GRPC: GRPC{
			Port: getEnv("GRPC_PORT", "50051"),
		},
		Database: Database{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnv("DB_PORT", "5432"),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			DBName:   getEnv("DB_NAME", "trading"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Kafka: Kafka{
			Brokers:           []string{getEnv("KAFKA_BROKER", "localhost:9092")},
			GroupID:           getEnv("KAFKA_GROUP_ID", "trading"),
			AccountTopic:      getEnv("KAFKA_ACCOUNT_TOPIC", "origination.account"),
			DepositEventTopic: getEnv("KAFKA_DEPOSIT_EVENT_TOPIC", "partnerconsumer.deposit"),
			DepositFeedTopic:  getEnv("KAFKA_DEPOSIT_FEED_TOPIC", "feed.deposit"),
			OrderEventTopic:   getEnv("KAFKA_ORDER_EVENT_TOPIC", "partnerconsumer.order"),
		},
		Dependencies: DependenciesConfig{
			PartnerProxy: Dependency{
				Address: getEnv("PARTNER_PROXY_ADDRESS", "localhost:50052"),
				Timeout: getEnv("PARTNER_PROXY_TIMEOUT", "5s"),
			},
		},
		Logger: Logger{
			Level:      getEnv("LOG_LEVEL", "info"),
			Encoding:   getEnv("LOG_ENCODING", "json"),
			OutputPath: getEnv("LOG_OUTPUT_PATH", "stdout"),
		},
	}
}

// getEnv gets the value of an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}
