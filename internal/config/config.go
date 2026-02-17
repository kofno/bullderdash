package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds application configuration
type Config struct {
	RedisAddr             string
	RedisUsername         string
	RedisPassword         string
	RedisDB               int
	RedisSentinelMaster   string
	RedisSentinelAddrs    []string
	RedisSentinelUsername string
	RedisSentinelPassword string
	ServerPort            string
	QueuePrefix           string
	MetricsPollSeconds    int
	LogLevel              string
}

// Load reads configuration from environment variables with sensible defaults
func Load() *Config {
	return &Config{
		RedisAddr:             getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		RedisUsername:         getEnv("REDIS_USERNAME", ""),
		RedisPassword:         getEnv("REDIS_PASSWORD", ""),
		RedisDB:               getEnvInt("REDIS_DB", 0),
		RedisSentinelMaster:   getEnv("REDIS_SENTINEL_MASTER", ""),
		RedisSentinelAddrs:    getEnvList("REDIS_SENTINEL_ADDRS"),
		RedisSentinelUsername: getEnv("REDIS_SENTINEL_USERNAME", ""),
		RedisSentinelPassword: getEnv("REDIS_SENTINEL_PASSWORD", ""),
		ServerPort:            getEnv("SERVER_PORT", "8080"),
		QueuePrefix:           getEnv("QUEUE_PREFIX", "bull"),
		MetricsPollSeconds:    getEnvInt("METRICS_POLL_SECONDS", 10),
		LogLevel:              getEnv("LOG_LEVEL", "info"),
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

func getEnvList(key string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
