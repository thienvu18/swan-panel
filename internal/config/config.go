package config

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Config holds all operational configuration parameters for SwanPanel
type Config struct {
	Port           int
	DBPath         string
	JWTSecret      string
	VICISocket     string
	DNSPort        int
	EnableDNS      bool
	Environment    string
	LogLevel       string
	AllowedOrigins []string
	PKIDataDir     string
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

	env := os.Getenv("APP_ENV")
	if env == "" {
		env = os.Getenv("ENVIRONMENT")
	}
	if env == "" {
		env = "development"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		if env == "production" {
			slog.Warn("SECURITY ALERT: JWT_SECRET environment variable is missing in production! Generating secure ephemeral random key.")
			jwtSecret = generateSecureRandomKey(32)
		} else {
			jwtSecret = "swanpanel-secure-default-jwt-secret-key-change-in-prod"
		}
	}

	viciSocket := os.Getenv("VICI_SOCKET")
	if viciSocket == "" {
		viciSocket = "/var/run/charon.vici"
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	allowedOrigins := parseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS"))

	pkiDataDir := os.Getenv("PKI_DATA_DIR")
	if pkiDataDir == "" {
		pkiDataDir = "/var/lib/swan-panel"
	}

	cfg := &Config{
		Port:           port,
		DBPath:         dbPath,
		JWTSecret:      jwtSecret,
		VICISocket:     viciSocket,
		DNSPort:        dnsPort,
		EnableDNS:      enableDNS,
		Environment:    env,
		LogLevel:       logLevel,
		AllowedOrigins: allowedOrigins,
		PKIDataDir:     pkiDataDir,
	}

	return cfg
}

func parseAllowedOrigins(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	var origins []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			origins = append(origins, trimmed)
		}
	}
	return origins
}

func generateSecureRandomKey(length int) string {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		slog.Error("Failed to read secure random bytes for JWT secret", "error", err)
		return "swanpanel-fallback-secure-crypto-random-key"
	}
	return hex.EncodeToString(b)
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
