package collab

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ObserverCallbacks holds the event handlers for an observer session.
type ObserverCallbacks struct {
	// OnToken is called for each streaming token received.
	OnToken func(token string)
	// OnToolStart is called when a tool begins executing.
	OnToolStart func(toolName string)
	// OnToolDone is called when a tool finishes.
	OnToolDone func(toolName string)
	// OnComplete is called when the current generation turn ends.
	OnComplete func()
}

// ObserveSession connects to a relay SSE endpoint and dispatches events until
// the session ends or the connection is closed. url may be a full URL
// (http://host:port/stream) or just host:port (stream path appended automatically).
func ObserveSession(url string, cb ObserverCallbacks) error {
	// Normalise URL.
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		url = "http://" + url
	}
	if !strings.HasSuffix(url, "/stream") && !strings.Contains(url, "/stream?") {
		url = strings.TrimRight(url, "/") + "/stream"
	}

	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return fmt.Errorf("connect to relay %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("relay returned HTTP %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")

		var event Event
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		switch event.Type {
		case EventConnected:
			// Handshake acknowledgement — no action needed.
		case EventToken:
			if cb.OnToken != nil {
				cb.OnToken(event.Content)
			}
		case EventToolStart:
			if cb.OnToolStart != nil {
				cb.OnToolStart(event.Content)
			}
		case EventToolDone:
			if cb.OnToolDone != nil {
				cb.OnToolDone(event.Content)
			}
		case EventComplete:
			if cb.OnComplete != nil {
				cb.OnComplete()
			}
			// Don't return — more turns may follow in the same session.
		}
	}
	return scanner.Err()
}
