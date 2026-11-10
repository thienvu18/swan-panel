package config

import (
	"log/slog"
	"os"
	"strconv"
)

// Config holds all operational configuration parameters for SwanPanel
type Config struct {
	Port        int
	DBPath      string
	JWTSecret   string
	VICISocket  string
	DNSPort     int
	EnableDNS   bool
	Environment string
	LogLevel    string
}

// Load reads configuration from environment variables or falls back to safe defaults
func Load() *Config {
	port := getEnvAsInt("PORT", 8080)
	dnsPort := getEnvAsInt("DNS_PORT", 5353) // Default to 5353 for rootless dev; map to 53 in production
	enableDNS := getEnvAsBool("ENABLE_DNS", true)

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "swanpanel.db"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "swanpanel-secure-default-jwt-secret-key-change-in-prod"
	}

	viciSocket := os.Getenv("VICI_SOCKET")
	if viciSocket == "" {
		viciSocket = "/var/run/charon.vici"
	}

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = os.Getenv("ENVIRONMENT")
	}
	if env == "" {
		env = "development"
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	cfg := &Config{
		Port:        port,
		DBPath:      dbPath,
		JWTSecret:   jwtSecret,
		VICISocket:  viciSocket,
		DNSPort:     dnsPort,
		EnableDNS:   enableDNS,
		Environment: env,
		LogLevel:    logLevel,
	}

	return cfg
}

func getEnvAsInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
		slog.Warn("Failed to parse integer environment variable, using fallback default", "key", key, "value", val, "default", defaultVal)
	}
	return defaultVal
}

func getEnvAsBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			return b
		}
		slog.Warn("Failed to parse boolean environment variable, using fallback default", "key", key, "value", val, "default", defaultVal)
	}
	return defaultVal
}
