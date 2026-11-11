package logger

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
)

// LogEntry represents an in-memory structured log event for UI consumption
type LogEntry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Scope     string `json:"scope"`
	Tag1      string `json:"tag1"`
	Tag2      string `json:"tag2"`
	Message   string `json:"message"`
}

var (
	logMu       sync.RWMutex
	logBuffer   []LogEntry
	maxLogs     = 1000
	LevelVar    = new(slog.LevelVar)
)

// AddLogEntry records a log entry in the ring buffer
func AddLogEntry(entry LogEntry) {
	logMu.Lock()
	defer logMu.Unlock()
	if len(logBuffer) >= maxLogs {
		logBuffer = logBuffer[1:]
	}
	logBuffer = append(logBuffer, entry)
}

// GetRecentLogs retrieves a copy of recent logs
func GetRecentLogs() []LogEntry {
	logMu.RLock()
	defer logMu.RUnlock()
	res := make([]LogEntry, len(logBuffer))
	copy(res, logBuffer)
	return res
}

// BufferedHandler wraps a slog.Handler and stores entries into the in-memory ring buffer
type BufferedHandler struct {
	inner slog.Handler
}

func (h *BufferedHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *BufferedHandler) Handle(ctx context.Context, r slog.Record) error {
	var tag1, tag2 string
	attrs := make([]string, 0)

	r.Attrs(func(a slog.Attr) bool {
		val := fmt.Sprintf("%v", a.Value.Any())
		attrs = append(attrs, fmt.Sprintf("%s=%s", a.Key, val))
		switch a.Key {
		case "ip", "client_ip":
			tag1 = val
		case "path", "method", "service":
			if tag2 == "" {
				tag2 = val
			}
		case "user_id", "email", "hostname":
			tag1 = val
		case "status":
			tag2 = val
		}
		return true
	})

	if tag1 == "" && len(attrs) > 0 {
		tag1 = attrs[0]
	}
	if tag2 == "" && len(attrs) > 1 {
		tag2 = attrs[1]
	}

	scope := strings.Join(attrs, " ")

	entry := LogEntry{
		Timestamp: r.Time.Format("15:04:05"),
		Level:     strings.ToLower(r.Level.String()),
		Scope:     scope,
		Tag1:      tag1,
		Tag2:      tag2,
		Message:   r.Message,
	}
	AddLogEntry(entry)

	return h.inner.Handle(ctx, r)
}

func (h *BufferedHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &BufferedHandler{inner: h.inner.WithAttrs(attrs)}
}

func (h *BufferedHandler) WithGroup(name string) slog.Handler {
	return &BufferedHandler{inner: h.inner.WithGroup(name)}
}

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

	bufferedHandler := &BufferedHandler{inner: handler}
	logger := slog.New(bufferedHandler)
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
