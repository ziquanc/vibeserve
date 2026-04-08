package web

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"time"

	"nhooyr.io/websocket"

	"github.com/vibeserve/vibeserve/internal/engine"
)

// wsMessage is the JSON envelope sent to WebSocket clients.
type wsMessage struct {
	Type string `json:"type"`
	Data any    `json:"data"`
	Time string `json:"time"`
}

// WSHub manages WebSocket connections and broadcasts Bus events.
type WSHub struct {
	bus   *engine.Bus
	mu    sync.RWMutex
	conns map[*websocket.Conn]context.CancelFunc
}

// NewWSHub creates a hub and subscribes to relevant Bus events.
func NewWSHub(bus *engine.Bus) *WSHub {
	h := &WSHub{
		bus:   bus,
		conns: make(map[*websocket.Conn]context.CancelFunc),
	}

	// Subscribe to HTTP events for the live trace tab.
	bus.Subscribe(engine.EventHTTPRequestReceived, func(e engine.Event) {
		h.broadcast(wsMessage{
			Type: "HTTP_REQUEST",
			Data: e.Data,
			Time: time.Now().UTC().Format(time.RFC3339Nano),
		})
	})

	bus.Subscribe(engine.EventHTTPResponseSent, func(e engine.Event) {
		h.broadcast(wsMessage{
			Type: "HTTP_RESPONSE",
			Data: e.Data,
			Time: time.Now().UTC().Format(time.RFC3339Nano),
		})
	})

	// Subscribe to route/schema change events for live updates.
	for _, et := range []engine.EventType{
		engine.EventRouteAdded,
		engine.EventRouteUpdated,
		engine.EventRouteRemoved,
		engine.EventSchemaAltered,
		engine.EventScriptLoaded,
		engine.EventDataSeeded,
		engine.EventSnapshotCreated,
		engine.EventSnapshotRestored,
	} {
		eventType := et // capture loop variable
		bus.Subscribe(eventType, func(e engine.Event) {
			h.broadcast(wsMessage{
				Type: string(eventType),
				Data: e.Data,
				Time: time.Now().UTC().Format(time.RFC3339Nano),
			})
		})
	}

	return h
}

// HandleWS is the HTTP handler that upgrades to WebSocket.
func (h *WSHub) HandleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		InsecureSkipVerify: true, // Allow connections from any origin (local dev tool).
	})
	if err != nil {
		log.Printf("[console/ws] accept failed: %v", err)
		return
	}

	ctx, cancel := context.WithCancel(r.Context())

	h.mu.Lock()
	h.conns[conn] = cancel
	h.mu.Unlock()

	log.Printf("[console/ws] client connected (%d total)", h.count())

	// Send a welcome message.
	welcome := wsMessage{
		Type: "CONNECTED",
		Data: map[string]any{"message": "VibeServe console connected"},
		Time: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if data, err := json.Marshal(welcome); err == nil {
		_ = conn.Write(ctx, websocket.MessageText, data)
	}

	// Read loop — keeps the connection alive and detects disconnects.
	// We do not expect messages from the client, but we must drain reads.
	for {
		_, _, err := conn.Read(ctx)
		if err != nil {
			break
		}
	}

	h.remove(conn)
	conn.Close(websocket.StatusNormalClosure, "bye")
	log.Printf("[console/ws] client disconnected (%d remaining)", h.count())
}

// broadcast sends a message to all connected WebSocket clients.
func (h *WSHub) broadcast(msg wsMessage) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range h.conns {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
			cancel()
			// Connection is dead — schedule removal.
			go h.remove(conn)
			continue
		}
		cancel()
	}
}

// remove cleans up a connection from the hub.
func (h *WSHub) remove(conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cancel, ok := h.conns[conn]; ok {
		cancel()
		delete(h.conns, conn)
	}
}

// count returns the number of active connections.
func (h *WSHub) count() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.conns)
}
