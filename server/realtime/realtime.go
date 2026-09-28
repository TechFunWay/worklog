// Package realtime provides a per-user Server-Sent Events (SSE) broker so
// apps can push live updates to the browser without pulling. It is a
// framework-level generalization of the reminder app's realtime broker: one
// broker, N user channels, buffered subscriber queues, heartbeat keep-alive.
//
// The framework wires a default endpoint at GET /api/realtime (auth required).
// Apps publish from any goroutine or gin handler:
//
//	realtime.Publish(userID, "todo.updated", gin.H{"id": id})
//
// and the frontend subscribes with EventSource:
//
//	const es = new EventSource('/api/realtime', { withCredentials: false })
//	es.addEventListener('message', (e) => { ... })  // data is a JSON Event
//
// For an app-local topic namespace, create a dedicated Broker and register its
// Handler on your own route.
package realtime

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// Event is the JSON payload delivered on the SSE stream. Data is
// app-defined and may be nil.
type Event struct {
	Type   string      `json:"type"`
	Data   interface{} `json:"data,omitempty"`
	SentAt time.Time   `json:"sent_at"`
}

// Broker tracks which users have open streams and fans events out to them.
// Create a dedicated Broker per topic namespace, or use the package-level
// Default broker.
type Broker struct {
	mu      sync.RWMutex
	clients map[uint]map[chan Event]struct{}
}

// Default is the broker backing GET /api/realtime. Apps that only need one
// event stream can use it directly.
var Default = &Broker{clients: make(map[uint]map[chan Event]struct{})}

func NewBroker() *Broker {
	return &Broker{clients: make(map[uint]map[chan Event]struct{})}
}

// Subscribe registers a buffered channel for the user and returns it together
// with the unsubscribe function. Slow clients drop the oldest buffered event
// rather than blocking the publisher.
func (b *Broker) Subscribe(userID uint) (<-chan Event, func()) {
	events := make(chan Event, 8)
	b.mu.Lock()
	if b.clients[userID] == nil {
		b.clients[userID] = make(map[chan Event]struct{})
	}
	b.clients[userID][events] = struct{}{}
	b.mu.Unlock()

	return events, func() {
		b.mu.Lock()
		delete(b.clients[userID], events)
		if len(b.clients[userID]) == 0 {
			delete(b.clients, userID)
		}
		b.mu.Unlock()
	}
}

// Publish delivers the event to every open stream of the user. It is safe to
// call from any goroutine; with no subscribers it is a no-op.
func (b *Broker) Publish(userID uint, event Event) {
	if event.SentAt.IsZero() {
		event.SentAt = time.Now()
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	for client := range b.clients[userID] {
		select {
		case client <- event:
		default:
			// Keep the newest state change if a slow browser has filled its
			// buffer. The browser refreshes canonical data after events.
			select {
			case <-client:
			default:
			}
			select {
			case client <- event:
			default:
			}
		}
	}
}

// Publish sends an event of the given type to the user on the Default broker.
func Publish(userID uint, eventType string, data interface{}) {
	Default.Publish(userID, Event{Type: eventType, Data: data})
}

// Handler returns a gin.HandlerFunc that streams the broker's events to the
// authenticated caller as an SSE response. Register it on an auth-protected
// route; the userID is read from the middleware's context key.
func (b *Broker) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"message": "当前连接不支持实时推送"})
			return
		}

		c.Header("Content-Type", "text/event-stream; charset=utf-8")
		c.Header("Cache-Control", "no-cache, no-transform")
		c.Header("Connection", "keep-alive")
		c.Header("X-Accel-Buffering", "no")
		c.Status(http.StatusOK)

		events, unsubscribe := b.Subscribe(c.GetUint("userID"))
		defer unsubscribe()

		ready := Event{Type: "connected", SentAt: time.Now()}
		if err := writeSSE(c.Writer, ready); err != nil {
			return
		}
		flusher.Flush()

		heartbeat := time.NewTicker(20 * time.Second)
		defer heartbeat.Stop()
		for {
			select {
			case <-c.Request.Context().Done():
				return
			case event := <-events:
				if err := writeSSE(c.Writer, event); err != nil {
					return
				}
				flusher.Flush()
			case <-heartbeat.C:
				if _, err := fmt.Fprint(c.Writer, ": heartbeat\n\n"); err != nil {
					return
				}
				flusher.Flush()
			}
		}
	}
}

// Handler is the SSE stream of the Default broker, authenticated by the
// framework's RequireAuth middleware.
func Handler() gin.HandlerFunc {
	return Default.Handler()
}

func writeSSE(w http.ResponseWriter, event Event) error {
	data, err := json.Marshal(event)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: message\ndata: %s\n\n", data)
	return err
}
