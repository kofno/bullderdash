package config

import (
	"os"
	"strconv"
)

// Config holds application configuration
type Config struct {
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	ServerPort    string
	QueuePrefix   string
	LogLevel      string
}

// Load reads configuration from environment variables with sensible defaults
func Load() *Config {
	return &Config{
		RedisAddr:     getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),
		ServerPort:    getEnv("SERVER_PORT", "8080"),
		QueuePrefix:   getEnv("QUEUE_PREFIX", "bull"),
		LogLevel:      getEnv("LOG_LEVEL", "info"),
	}
}

func getEnv(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return fallback
}
