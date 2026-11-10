package logger

import (
	"log/slog"
	"os"
	"strings"
)

// LevelVar holds the dynamic logging level for SwanPanel
var LevelVar = new(slog.LevelVar)

// InitLogger configures the global slog logger based on the configured log level and environment
func InitLogger(levelStr string, environment string) *slog.Logger {
	level := parseLevel(levelStr)
	LevelVar.Set(level)

	opts := &slog.HandlerOptions{
		Level: LevelVar,
	}

	var handler slog.Handler
	if strings.EqualFold(environment, "production") {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}

// SetLevel dynamically changes the active log level at runtime
func SetLevel(levelStr string) {
	LevelVar.Set(parseLevel(levelStr))
}

func parseLevel(levelStr string) slog.Level {
	switch strings.ToUpper(strings.TrimSpace(levelStr)) {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	case "INFO":
		fallthrough
	default:
		return slog.LevelInfo
	}
}
