package realtime

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Event represents a real-time event dispatched to connected dashboards
type Event struct {
	Type      string      `json:"type"` // 'session_update', 'gateway_status', 'log_event', 'device_change'
	Payload   interface{} `json:"payload"`
	Timestamp int64       `json:"timestamp"`
}

// Broker manages active Server-Sent Event (SSE) subscriber connections
type Broker struct {
	clients        map[chan Event]bool
	newClients     chan chan Event
	closingClients chan chan Event
	broadcast      chan Event
	mu             sync.RWMutex
}

var DefaultBroker = NewBroker()

func NewBroker() *Broker {
	b := &Broker{
		clients:        make(map[chan Event]bool),
		newClients:     make(chan chan Event),
		closingClients: make(chan chan Event),
		broadcast:      make(chan Event, 100),
	}
	go b.listen()
	return b
}

func (b *Broker) listen() {
	for {
		select {
		case s := <-b.newClients:
			b.mu.Lock()
			b.clients[s] = true
			total := len(b.clients)
			b.mu.Unlock()
			slog.Debug("New SSE client subscribed to event stream", "active_subscribers", total)
		case s := <-b.closingClients:
			b.mu.Lock()
			delete(b.clients, s)
			close(s)
			total := len(b.clients)
			b.mu.Unlock()
			slog.Debug("SSE client unsubscribed from event stream", "active_subscribers", total)
		case event := <-b.broadcast:
			b.mu.RLock()
			for clientChan := range b.clients {
				select {
				case clientChan <- event:
				default:
					slog.Warn("Skipping saturated SSE client buffer", "event_type", event.Type)
				}
			}
			b.mu.RUnlock()
		}
	}
}

// Broadcast sends a real-time event to all connected dashboard subscribers
func (b *Broker) Broadcast(eventType string, payload interface{}) {
	slog.Debug("Broadcasting realtime event", "type", eventType)
	b.broadcast <- Event{
		Type:      eventType,
		Payload:   payload,
		Timestamp: time.Now().UnixMilli(),
	}
}

// SSEHandler serves the Server-Sent Events endpoint /api/realtime/stream
func (b *Broker) SSEHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientChan := make(chan Event, 20)
		b.newClients <- clientChan

		defer func() {
			b.closingClients <- clientChan
		}()

		// Set SSE response headers
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("Transfer-Encoding", "chunked")

		// Periodic keepalive ping every 15 seconds to prevent router/proxy timeouts
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()

		c.Writer.WriteHeader(http.StatusOK)
		c.Writer.Flush()

		ctx := c.Request.Context()
		for {
			select {
			case <-ctx.Done():
				slog.Debug("SSE client stream connection terminated", "ip", c.ClientIP())
				return
			case <-ticker.C:
				_, err := c.Writer.Write([]byte(": keepalive\n\n"))
				if err != nil {
					slog.Debug("SSE keepalive write failed, disconnecting client", "ip", c.ClientIP())
					return
				}
				c.Writer.Flush()
			case event, ok := <-clientChan:
				if !ok {
					slog.Debug("SSE client channel closed", "ip", c.ClientIP())
					return
				}
				data, err := json.Marshal(event)
				if err != nil {
					slog.Error("Failed to serialize SSE event payload to JSON", "error", err)
					continue
				}
				_, err = c.Writer.Write([]byte("data: " + string(data) + "\n\n"))
				if err != nil {
					slog.Debug("SSE data write failed, disconnecting client", "ip", c.ClientIP())
					return
				}
				c.Writer.Flush()
			}
		}
	}
}
