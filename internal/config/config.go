// Package config handles environment-driven configuration loading for Nexus AI Gateway.
package config

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config aggregates all subsystem configurations.
type Config struct {
	Server        ServerConfig
	Database      DatabaseConfig
	Redis         RedisConfig
	Kafka         KafkaConfig
	Observability ObservabilityConfig
	Providers     ProvidersConfig
}

// ServerConfig defines HTTP server settings.
type ServerConfig struct {
	Port            int    `json:"port"`
	ReadTimeoutSec  int    `json:"read_timeout_sec"`
	WriteTimeoutSec int    `json:"write_timeout_sec"`
	IdleTimeoutSec  int    `json:"idle_timeout_sec"`
	Env             string `json:"env"`
}

// DatabaseConfig holds PostgreSQL connection parameters.
type DatabaseConfig struct {
	URL            string `json:"url"`
	MaxConns       int    `json:"max_conns"`
	MinConns       int    `json:"min_conns"`
	MigrationsPath string `json:"migrations_path"`
}

// RedisConfig holds Redis connectivity options.
type RedisConfig struct {
	URL      string `json:"url"`
	Password string `json:"password"`
	DB       int    `json:"db"`
}

// KafkaConfig holds event streaming configuration.
type KafkaConfig struct {
	Brokers []string `json:"brokers"`
	Topic   string   `json:"topic"`
}

// ObservabilityConfig holds tracing, metrics, and logging flags.
type ObservabilityConfig struct {
	OTLPEndpoint   string `json:"otlp_endpoint"`
	ServiceName    string `json:"service_name"`
	MetricsEnabled bool   `json:"metrics_enabled"`
	TracingEnabled bool   `json:"tracing_enabled"`
	LogLevel       string `json:"log_level"`
}

// ProvidersConfig holds base URLs and fallback options for providers (local testing & defaults).
type ProvidersConfig struct {
	OpenAIBaseURL    string `json:"openai_base_url"`
	AnthropicBaseURL string `json:"anthropic_base_url"`
	GeminiBaseURL    string `json:"gemini_base_url"`
}

// Load populates Config from environment variables with sensible defaults.
func Load() (*Config, error) {
	loadDotEnv(".env")

	dbURL := getEnvOrDefault("DATABASE_URL", "postgres://postgres:postgres@localhost:5432/nexus?sslmode=disable")
	redisURL := getEnvOrDefault("REDIS_URL", "redis://localhost:6379/0")

	kafkaBrokersRaw := getEnvOrDefault("KAFKA_BROKERS", "localhost:9092")
	var brokers []string
	for _, b := range strings.Split(kafkaBrokersRaw, ",") {
		if trimmed := strings.TrimSpace(b); trimmed != "" {
			brokers = append(brokers, trimmed)
		}
	}

	cfg := &Config{
		Server: ServerConfig{
			Port:            getEnvInt("SERVER_PORT", 8080),
			ReadTimeoutSec:  getEnvInt("SERVER_READ_TIMEOUT_SEC", 30),
			WriteTimeoutSec: getEnvInt("SERVER_WRITE_TIMEOUT_SEC", 60),
			IdleTimeoutSec:  getEnvInt("SERVER_IDLE_TIMEOUT_SEC", 120),
			Env:             getEnvOrDefault("ENV", "local"),
		},
		Database: DatabaseConfig{
			URL:            dbURL,
			MaxConns:       getEnvInt("DATABASE_MAX_CONNS", 25),
			MinConns:       getEnvInt("DATABASE_MIN_CONNS", 5),
			MigrationsPath: getEnvOrDefault("MIGRATIONS_PATH", "migrations"),
		},
		Redis: RedisConfig{
			URL:      redisURL,
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Kafka: KafkaConfig{
			Brokers: brokers,
			Topic:   getEnvOrDefault("KAFKA_TOPIC_REQUESTS", "ai.requests"),
		},
		Observability: ObservabilityConfig{
			OTLPEndpoint:   getEnvOrDefault("OTEL_EXPORTER_OTLP_ENDPOINT", "http://localhost:4318"),
			ServiceName:    getEnvOrDefault("OTEL_SERVICE_NAME", "nexus-gateway-api"),
			MetricsEnabled: getEnvBool("METRICS_ENABLED", true),
			TracingEnabled: getEnvBool("TRACING_ENABLED", true),
			LogLevel:       getEnvOrDefault("LOG_LEVEL", "info"),
		},
		Providers: ProvidersConfig{
			OpenAIBaseURL:    getEnvOrDefault("OPENAI_BASE_URL", "https://api.openai.com/v1"),
			AnthropicBaseURL: getEnvOrDefault("ANTHROPIC_BASE_URL", "https://api.anthropic.com"),
			GeminiBaseURL:    getEnvOrDefault("GEMINI_BASE_URL", "https://generativelanguage.googleapis.com"),
		},
	}

	if cfg.Database.URL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.Redis.URL == "" {
		return nil, fmt.Errorf("REDIS_URL is required")
	}

	return cfg, nil
}

func getEnvOrDefault(key, defaultVal string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	return val
}

func getEnvInt(key string, defaultVal int) int {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return i
}

func getEnvBool(key string, defaultVal bool) bool {
	val := os.Getenv(key)
	if val == "" {
		return defaultVal
	}
	b, err := strconv.ParseBool(val)
	if err != nil {
		return defaultVal
	}
	return b
}

func loadDotEnv(filename string) {
	file, err := os.Open(filename)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		// Strip surrounding quotes if present
		val = strings.Trim(val, `"'`)
		if os.Getenv(key) == "" {
			_ = os.Setenv(key, val)
		}
	}
}
