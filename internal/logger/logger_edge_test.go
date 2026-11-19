package logger

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestBufferedHandlerCapturesRelevantAuditContextAndIsolatesReturnedLogs(t *testing.T) {
	logMu.Lock()
	previousBuffer, previousMax := logBuffer, maxLogs
	logBuffer = nil
	maxLogs = 3
	logMu.Unlock()
	t.Cleanup(func() {
		logMu.Lock()
		logBuffer, maxLogs = previousBuffer, previousMax
		logMu.Unlock()
	})
	var output bytes.Buffer
	handler := &BufferedHandler{inner: slog.NewTextHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug})}
	slogger := slog.New(handler)
	for i, attrs := range [][]any{
		{"client_ip", "192.0.2.1", "path", "/api/users"},
		{"user_id", "admin1", "status", "denied"},
		{"email", "someone@example.org", "method", "POST"},
		{"hostname", "vpn-client", "service", "charon"},
	} {
		slogger.Log(context.Background(), slog.LevelInfo, "audit-event", attrs...)
		records := GetRecentLogs()
		if len(records) == 0 {
			t.Fatalf("iteration %d lost audit logs", i)
		}
	}
	entries := GetRecentLogs()
	if len(entries) != 3 || entries[0].Tag1 != "admin1" || entries[0].Tag2 != "denied" ||
		entries[1].Tag1 != "someone@example.org" || entries[1].Tag2 != "POST" ||
		entries[2].Tag1 != "vpn-client" || entries[2].Tag2 != "charon" {
		t.Fatalf("audit record mapping/ring eviction incorrect: %+v", entries)
	}
	entries[0].Tag1 = "modified"
	if GetRecentLogs()[0].Tag1 == "modified" {
		t.Fatal("returned logs exposed mutable shared memory")
	}
	if !strings.Contains(output.String(), "audit-event") {
		t.Fatal("underlying logger did not receive event")
	}
	if !handler.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("enabled level was lost")
	}
	if _, ok := handler.WithAttrs([]slog.Attr{slog.String("component", "api")}).(*BufferedHandler); !ok {
		t.Fatal("WithAttrs did not preserve buffered handler")
	}
	if _, ok := handler.WithGroup("request").(*BufferedHandler); !ok {
		t.Fatal("WithGroup did not preserve buffered handler")
	}
}

func TestLoggerDynamicLevelAndDefaultTags(t *testing.T) {
	logger := InitLogger("ERROR", "production")
	t.Cleanup(func() { SetLevel("INFO") })
	if LevelVar.Level() != slog.LevelError {
		t.Fatal("initial logging level incorrect")
	}
	SetLevel("warning")
	if LevelVar.Level() != slog.LevelWarn {
		t.Fatal("warning synonym not applied")
	}
	SetLevel("DEBUG")
	if !logger.Enabled(context.Background(), slog.LevelDebug) {
		t.Fatal("live log level change not applied")
	}
	for _, tc := range []struct {
		level    string
		expected slog.Level
	}{
		{" warn ", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"ERROR", slog.LevelError},
		{"info", slog.LevelInfo},
		{"unknown", slog.LevelInfo},
	} {
		if got := parseLevel(tc.level); got != tc.expected {
			t.Errorf("level %q parsed %d; expected %d", tc.level, got, tc.expected)
		}
	}
	// A zero-attribute audit event must not manufacture identity fields.
	record := slog.NewRecord(time.Now(), slog.LevelInfo, "empty", 0)
	var sink bytes.Buffer
	handler := &BufferedHandler{inner: slog.NewTextHandler(&sink, nil)}
	if err := handler.Handle(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	entry := GetRecentLogs()[len(GetRecentLogs())-1]
	if entry.Tag1 != "" || entry.Tag2 != "" {
		t.Fatalf("empty metadata leaked another log's tags: %+v", entry)
	}
}

func TestBufferedHandlerIncludesPersistentWithAttrsContext(t *testing.T) {
	logMu.Lock()
	previous := logBuffer
	logBuffer = nil
	logMu.Unlock()
	t.Cleanup(func() {
		logMu.Lock()
		logBuffer = previous
		logMu.Unlock()
	})
	var sink bytes.Buffer
	handler := &BufferedHandler{inner: slog.NewTextHandler(&sink, nil)}
	withContext := slog.New(handler).With("user_id", "viewer-1").With("path", "/api/logs")
	withContext.Info("request inspected", "status", "allowed")
	entries := GetRecentLogs()
	if len(entries) != 1 {
		t.Fatalf("expected one contextual log, got %d", len(entries))
	}
	if entries[0].Tag1 != "viewer-1" || entries[0].Tag2 != "allowed" {
		t.Fatalf("persistent context was lost: %+v", entries[0])
	}
	if !strings.Contains(entries[0].Scope, "path=/api/logs") {
		t.Fatalf("persistent path was missing from scope: %s", entries[0].Scope)
	}
}
