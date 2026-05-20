package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL string
	RedisAddr   string
	HTTPListen  string
	LogLevel    slog.Level
}

func Load() (Config, error) {
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	return Config{
		DatabaseURL: dbURL,
		RedisAddr:   getEnv("REDIS_ADDR", "127.0.0.1:6379"),
		HTTPListen:  getEnv("HTTP_LISTEN", "127.0.0.1:8080"),
		LogLevel:    parseLevel(getEnv("LOG_LEVEL", "info")),
	}, nil
}

func getEnv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
