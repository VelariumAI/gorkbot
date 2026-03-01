// Package collab implements Remote Session Sharing via Server-Sent Events (SSE).
// The Relay broadcasts streaming tokens and tool events over HTTP so that remote
// observers can watch a live Gorkbot session from any browser, curl, or a second
// gorkbot instance running with --join.
package collab

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"
)

// EventType classifies a relay broadcast.
type EventType string

const (
	EventToken     EventType = "token"
	EventToolStart EventType = "tool_start"
	EventToolDone  EventType = "tool_done"
	EventComplete  EventType = "complete"
	EventConnected EventType = "connected"
)

// Event is the JSON payload sent to SSE subscribers.
type Event struct {
	Type    EventType `json:"type"`
	Content string    `json:"content,omitempty"`
}

// Relay is a single-session SSE broadcast server.
// Start it with Start(), wire it into the orchestrator, call Stop() on exit.
type Relay struct {
	mu       sync.RWMutex
	clients  map[chan Event]struct{}
	listener net.Listener
	port     int
}

// NewRelay creates a Relay that will listen on port (0 = random available port).
func NewRelay(port int) *Relay {
	return &Relay{
		clients: make(map[chan Event]struct{}),
		port:    port,
	}
}

// Start binds the relay to its port and begins serving.
// Returns the SSE stream URL observers should connect to.
func (r *Relay) Start() (string, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/stream", r.handleSSE)
	mux.HandleFunc("/", r.handleInfo)

	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", r.port))
	if err != nil {
		return "", fmt.Errorf("relay listen: %w", err)
	}
	r.listener = ln
	r.port = ln.Addr().(*net.TCPAddr).Port

	server := &http.Server{Handler: mux}
	go func() {
		_ = server.Serve(ln) // errors suppressed after Stop()
	}()

	return fmt.Sprintf("http://localhost:%d/stream", r.port), nil
}

// Stop shuts down the relay server.
func (r *Relay) Stop() {
	if r.listener != nil {
		_ = r.listener.Close()
	}
}

// Port returns the actual port being listened on (useful when port=0).
func (r *Relay) Port() int { return r.port }

// SendToken broadcasts a streaming AI token to all connected observers.
func (r *Relay) SendToken(token string) {
	r.broadcast(Event{Type: EventToken, Content: token})
}

// SendToolStart signals that a tool has begun executing.
func (r *Relay) SendToolStart(toolName string) {
	r.broadcast(Event{Type: EventToolStart, Content: toolName})
}

// SendToolDone signals that a tool has finished.
func (r *Relay) SendToolDone(toolName string) {
	r.broadcast(Event{Type: EventToolDone, Content: toolName})
}

// SendComplete signals the end of the current generation turn.
func (r *Relay) SendComplete() {
	r.broadcast(Event{Type: EventComplete})
}

func (r *Relay) broadcast(event Event) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for ch := range r.clients {
		select {
		case ch <- event:
		default: // drop — slow observer
		}
	}
}

func (r *Relay) subscribe() chan Event {
	ch := make(chan Event, 128)
	r.mu.Lock()
	r.clients[ch] = struct{}{}
	r.mu.Unlock()
	return ch
}

func (r *Relay) unsubscribe(ch chan Event) {
	r.mu.Lock()
	delete(r.clients, ch)
	r.mu.Unlock()
}

func (r *Relay) handleSSE(w http.ResponseWriter, req *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ch := r.subscribe()
	defer r.unsubscribe(ch)

	// Send greeting immediately.
	connected, _ := json.Marshal(Event{Type: EventConnected})
	fmt.Fprintf(w, "data: %s\n\n", connected)
	flusher.Flush()

	for {
		select {
		case <-req.Context().Done():
			return
		case event := <-ch:
			data, _ := json.Marshal(event)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

func (r *Relay) handleInfo(w http.ResponseWriter, req *http.Request) {
	host := req.Host
	if host == "" {
		host = fmt.Sprintf("localhost:%d", r.port)
	}
	fmt.Fprintf(w,
		"Gorkbot Session Relay\n\nObserver URL:\n  http://%s/stream\n\nConnect with:\n  curl -N http://%s/stream\n  gorkbot --join %s\n",
		host, host, host,
	)
}
