package webui

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// WSClient represents a single WebSocket connection to a browser client.
type WSClient struct {
	hub  *WSHub
	conn *websocket.Conn
	send chan []byte
}

// WSHub manages all active WebSocket client connections and broadcasts messages to all.
type WSHub struct {
	clients    map[*WSClient]struct{}
	broadcast  chan []byte
	register   chan *WSClient
	unregister chan *WSClient
	mu         sync.RWMutex
}

// NewWSHub creates a new WebSocket hub.
func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[*WSClient]struct{}),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}
}

// Run starts the hub's main event loop. Must be called once in a goroutine.
func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = struct{}{}
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.mu.RLock()
			for client := range h.clients {
				// Non-blocking send: drop if client is slow
				select {
				case client.send <- message:
				default:
					// Client send buffer full, skip
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast encodes v as JSON and sends it to all connected clients.
func (h *WSHub) Broadcast(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}

	select {
	case h.broadcast <- data:
	default:
		// Broadcast buffer full, skip
	}
	return nil
}

// ClientCount returns the current number of connected clients.
func (h *WSHub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ServeWS handles a WebSocket upgrade request and manages the client connection.
func (h *WSHub) ServeWS(c *gin.Context) {
	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			// For Phase 3, allow same-origin connections
			return true
		},
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &WSClient{
		hub:  h,
		conn: conn,
		send: make(chan []byte, 64),
	}

	h.register <- client

	// Run reader and writer in separate goroutines
	go client.readPump()
	go client.writePump()
}

// readPump reads messages from the WebSocket connection.
func (c *WSClient) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetPongHandler(func(string) error {
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
			}
			return
		}
		// For now, we don't process incoming client messages
	}
}

// writePump writes messages to the WebSocket connection.
func (c *WSClient) writePump() {
	defer func() {
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				// Channel closed
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}
		}
	}
}
