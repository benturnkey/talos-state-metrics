package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr       string
	TalosEndpoint    string
	TalosConfigPath  string
	FullSyncInterval time.Duration
	ShutdownTimeout  time.Duration
	MinBackoff       time.Duration
	MaxBackoff       time.Duration
}

func FromEnv() Config {
	return Config{
		ListenAddr:       env("LISTEN_ADDR", ":8080"),
		TalosEndpoint:    env("TALOS_ENDPOINT", "127.0.0.1"),
		TalosConfigPath:  env("TALOS_CONFIG", "/var/run/talos/config"),
		FullSyncInterval: durationEnv("FULL_SYNC_INTERVAL_SECONDS", 15*time.Minute),
		ShutdownTimeout:  durationEnv("SHUTDOWN_TIMEOUT_SECONDS", 30*time.Second),
		MinBackoff:       durationEnv("WATCH_MIN_BACKOFF_SECONDS", time.Second),
		MaxBackoff:       durationEnv("WATCH_MAX_BACKOFF_SECONDS", 30*time.Second),
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func durationEnv(name string, fallback time.Duration) time.Duration {
	value := os.Getenv(name)
	if value == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(value)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}
