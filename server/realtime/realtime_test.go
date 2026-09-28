package realtime

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestPublishReachesSubscriber(t *testing.T) {
	broker := NewBroker()
	events, unsubscribe := broker.Subscribe(42)
	defer unsubscribe()

	go broker.Publish(42, Event{Type: "todo.updated", Data: map[string]int{"id": 1}})

	select {
	case event := <-events:
		if event.Type != "todo.updated" {
			t.Fatalf("event type = %q, want todo.updated", event.Type)
		}
		if event.SentAt.IsZero() {
			t.Fatal("Publish must stamp SentAt")
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber did not receive the published event")
	}
}

func TestPublishWithoutSubscribersIsNoop(t *testing.T) {
	// Must not block or panic with no subscribers.
	Default.Publish(1, Event{Type: "noop"})
}

func TestUnsubscribeRemovesUser(t *testing.T) {
	broker := NewBroker()
	events, unsubscribe := broker.Subscribe(7)
	unsubscribe()

	broker.Publish(7, Event{Type: "after.unsubscribe"})

	select {
	case event := <-events:
		t.Fatalf("unsubscribed channel received %v", event)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSlowSubscriberDropsInsteadOfBlocking(t *testing.T) {
	broker := NewBroker()
	_, unsubscribe := broker.Subscribe(9)
	defer unsubscribe()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			broker.Publish(9, Event{Type: "flood"})
		}
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}
}

func TestHandlerStreamsSSE(t *testing.T) {
	gin.SetMode(gin.TestMode)

	broker := NewBroker()
	router := gin.New()
	router.GET("/realtime", func(c *gin.Context) {
		c.Set("userID", uint(3))
		broker.Handler()(c)
	})

	server := httptest.NewServer(router)
	defer server.Close()

	resp, err := http.Get(server.URL + "/realtime")
	if err != nil {
		t.Fatalf("GET /realtime: %v", err)
	}
	defer resp.Body.Close()

	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content type = %q, want text/event-stream", ct)
	}

	buf := make([]byte, 512)
	if _, err := resp.Body.Read(buf); err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if !strings.Contains(string(buf), `"type":"connected"`) {
		t.Fatalf("stream did not start with connected event: %q", string(buf))
	}

	// Publish while the stream is open; the JSON payload must round-trip.
	broker.Publish(3, Event{Type: "ping", Data: map[string]string{"hello": "world"}})

	deadline := time.Now().Add(2 * time.Second)
	payload := make([]byte, 0, 1024)
	chunk := make([]byte, 256)
	for time.Now().Before(deadline) {
		n, _ := resp.Body.Read(chunk)
		payload = append(payload, chunk[:n]...)
		if strings.Contains(string(payload), `"type":"ping"`) {
			break
		}
	}
	if !strings.Contains(string(payload), `"type":"ping"`) {
		t.Fatalf("published event never reached the stream: %q", string(payload))
	}
	var event Event
	start := strings.Index(string(payload), "data: ")
	if start < 0 {
		t.Fatalf("no SSE data frame found: %q", string(payload))
	}
	if err := json.Unmarshal([]byte(string(payload)[start+6:strings.Index(string(payload)[start+6:], "\n")+start+6]), &event); err != nil {
		t.Fatalf("event payload is not JSON: %v (%q)", err, string(payload))
	}
}
