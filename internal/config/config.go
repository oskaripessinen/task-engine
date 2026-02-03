package config

import (
	"fmt"
	"os"
	"strconv"
)

const (
	defaultDBDSN             = "postgres://task:task@localhost:5432/task?sslmode=disable"
	defaultRedisAddr         = "localhost:6379"
	defaultAPIPort           = 8080
	defaultWorkerCount       = 4
	defaultWorkerMetricsPort = 9091
)

type Config struct {
	DBDSN             string
	RedisAddr         string
	APIPort           int
	WorkerCount       int
	WorkerMetricsPort int
}

func Load() (Config, error) {
	cfg := Config{
		DBDSN:             envOrDefault("DB_DSN", defaultDBDSN),
		RedisAddr:         envOrDefault("REDIS_ADDR", defaultRedisAddr),
		APIPort:           envIntOrDefault("API_PORT", defaultAPIPort),
		WorkerCount:       envIntOrDefault("WORKER_COUNT", defaultWorkerCount),
		WorkerMetricsPort: envIntOrDefault("WORKER_METRICS_PORT", defaultWorkerMetricsPort),
	}

	if cfg.DBDSN == "" {
		return Config{}, fmt.Errorf("DB_DSN is required")
	}
	if cfg.RedisAddr == "" {
		return Config{}, fmt.Errorf("REDIS_ADDR is required")
	}
	if cfg.APIPort <= 0 || cfg.APIPort > 65535 {
		return Config{}, fmt.Errorf("API_PORT must be between 1 and 65535")
	}
	if cfg.WorkerCount <= 0 {
		return Config{}, fmt.Errorf("WORKER_COUNT must be greater than 0")
	}
	if cfg.WorkerMetricsPort <= 0 || cfg.WorkerMetricsPort > 65535 {
		return Config{}, fmt.Errorf("WORKER_METRICS_PORT must be between 1 and 65535")
	}

	return cfg, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
