package realtime

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/swan-panel/swan-panel/internal/db"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func sseTestDatabase(t *testing.T) *gorm.DB {
	t.Helper()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "events.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := database.AutoMigrate(&db.User{}); err != nil {
		t.Fatal(err)
	}
	old := db.DB
	db.DB = database
	t.Cleanup(func() { db.DB = old; raw, _ := database.DB(); _ = raw.Close() })
	return database
}

func openTestEventStream(t *testing.T, broker *Broker, userID string) (*http.Response, *httptest.Server, context.CancelFunc) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/events", func(c *gin.Context) { c.Set("user_id", userID); broker.SSEHandler()(c) })
	srv := httptest.NewServer(router)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	req, err := http.NewRequestWithContext(ctx, "GET", srv.URL+"/events", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("SSE handler returned %d", resp.StatusCode)
	}
	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatal("SSE response was not streamed")
	}
	t.Cleanup(func() { cancel(); _ = resp.Body.Close(); srv.Close() })
	return resp, srv, cancel
}

func TestSSEStreamFiltersPrivilegedEventsAndStopsAfterRevocation(t *testing.T) {
	database := sseTestDatabase(t)
	user := db.User{ID: "stream-user", Email: "stream@example.invalid", Name: "Stream User", AuthType: "local", Status: "ACTIVE"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	broker := NewBroker()
	resp, _, cancel := openTestEventStream(t, broker, user.ID)
	reader := bufio.NewReader(resp.Body)
	broker.Broadcast("user_approved", map[string]any{"email": "top-secret@example.invalid"})
	broker.Broadcast("session_terminated", map[string]any{"ike_name": "private-session"})
	// An administrator-only event must be dropped; the next frame is an anonymous session refresh.
	line, err := reader.ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "session_update") || strings.Contains(line, "private-session") ||
		strings.Contains(line, "top-secret@example.invalid") {
		t.Fatalf("privileged information escaped SSE stream: %q", line)
	}
	if err := database.Model(&user).Update("status", "SUSPENDED").Error; err != nil {
		t.Fatal(err)
	}
	broker.Broadcast("session_terminated", map[string]any{"ike_name": "private-session"})
	// The stream must close on next event without leaking its payload.
	_, err = io.ReadAll(reader)
	if err != nil && err != context.Canceled && !strings.Contains(err.Error(), "context canceled") {
		t.Fatalf("SSE stream did not close after suspension: %v", err)
	}
	cancel()
}

func TestSSEAdministratorReceivesFullEventPayload(t *testing.T) {
	database := sseTestDatabase(t)
	admin := db.User{ID: "stream-admin", Email: "admin@example.invalid", Name: "Admin", AuthType: "local",
		Status: "ACTIVE", IsAdmin: true}
	if err := database.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	broker := NewBroker()
	resp, _, _ := openTestEventStream(t, broker, admin.ID)
	broker.Broadcast("user_approved", map[string]any{"email": "new-user@example.invalid"})
	line, err := bufio.NewReader(resp.Body).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "user_approved") || !strings.Contains(line, "new-user@example.invalid") {
		t.Fatalf("admin audit stream missing event: %q", line)
	}
}

func TestSSESubscriberUnknownAccountIsDisconnectedBeforeSensitiveEvent(t *testing.T) {
	database := sseTestDatabase(t)
	user := db.User{ID: "stream-deleted", Email: "gone@example.invalid", Name: "Deleted", AuthType: "local", Status: "ACTIVE"}
	if err := database.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	broker := NewBroker()
	resp, _, _ := openTestEventStream(t, broker, user.ID)
	if err := database.Delete(&user).Error; err != nil {
		t.Fatal(err)
	}
	broker.Broadcast("user_approved", map[string]any{"email": "should-not-leak@example.invalid"})
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(fmt.Errorf("read disconnected stream: %w", err))
	}
	if strings.Contains(string(content), "should-not-leak@example.invalid") {
		t.Fatal("deleted account received event")
	}
}
