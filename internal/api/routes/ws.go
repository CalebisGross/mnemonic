package routes

import (
	"context"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/appsprout/mnemonic/internal/events"
	"github.com/gorilla/websocket"
)

const maxWebSocketConns = 10

var activeWSConns atomic.Int32

// WebSocketMessage is the format for messages sent over the WebSocket.
type WebSocketMessage struct {
	Type      string      `json:"type"`
	Timestamp string      `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

// wsConn wraps a WebSocket connection with subscription management.
type wsConn struct {
	conn            *websocket.Conn
	subscriptionIDs []string
	eventChan       chan events.Event
	log             *slog.Logger
}

// allowedWSOrigins is the set of origins allowed to open WebSocket connections.
var allowedWSOrigins = map[string]bool{
	"http://localhost:3000": true,
	"http://localhost:8080": true,
	"http://127.0.0.1:3000": true,
	"http://127.0.0.1:8080": true,
	"http://localhost:9999": true,
	"http://127.0.0.1:9999": true,
}

// upgrader is the WebSocket upgrader with default settings.
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // Allow requests with no origin (e.g., CLI tools)
		}
		return allowedWSOrigins[origin]
	},
}

// HandleWebSocket returns an HTTP handler that upgrades connections to WebSocket.
// Subscribes to all event types on the event bus and broadcasts them to the client.
// Handles client disconnection and cleanup.
func HandleWebSocket(bus events.Bus, log *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Enforce connection limit
		if activeWSConns.Load() >= maxWebSocketConns {
			log.Warn("websocket connection limit reached", "max", maxWebSocketConns, "remote_addr", r.RemoteAddr)
			http.Error(w, "too many websocket connections", http.StatusServiceUnavailable)
			return
		}
		activeWSConns.Add(1)

		// Upgrade HTTP connection to WebSocket
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			activeWSConns.Add(-1)
			log.Error("websocket upgrade failed", "error", err, "remote_addr", r.RemoteAddr)
			return
		}
		defer conn.Close()
		defer activeWSConns.Add(-1)

		log.Info("websocket connection established", "remote_addr", r.RemoteAddr)

		// Create connection wrapper
		wsConn := &wsConn{
			conn:            conn,
			subscriptionIDs: []string{},
			eventChan:       make(chan events.Event, 100),
			log:             log,
		}

		// Subscribe to all event types
		eventTypes := []string{
			events.TypeRawMemoryCreated,
			events.TypeMemoryEncoded,
			events.TypeMemoryAccessed,
			events.TypeConsolidationStarted,
			events.TypeConsolidationCompleted,
			events.TypeQueryExecuted,
			events.TypeMetaCycleCompleted,
			events.TypeDreamCycleCompleted,
			events.TypeSystemHealth,
			events.TypeWatcherEvent,
			events.TypeEpisodeClosed,
		}

		for _, eventType := range eventTypes {
			subID := bus.Subscribe(eventType, func(ctx context.Context, evt events.Event) error {
				select {
				case wsConn.eventChan <- evt:
				default:
					log.Warn("websocket event channel full, dropping event", "event_type", evt.EventType())
				}
				return nil
			})
			wsConn.subscriptionIDs = append(wsConn.subscriptionIDs, subID)
		}

		log.Debug("websocket subscribed to all event types", "subscription_count", len(wsConn.subscriptionIDs))

		// Set up ping/pong to keep connection alive and detect disconnection
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		conn.SetPongHandler(func(string) error {
			_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			return nil
		})

		// Start goroutine to read from client and detect disconnect
		clientDone := make(chan struct{})
		go func() {
			defer close(clientDone)
			for {
				_, _, err := conn.ReadMessage()
				if err != nil {
					if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
						log.Warn("websocket error", "error", err)
					}
					return
				}
				// Reset read deadline on each message to keep connection alive
				_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
			}
		}()

		// Send events to client or wait for disconnect
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-clientDone:
				// Client disconnected
				log.Info("websocket client disconnected", "remote_addr", r.RemoteAddr)
				wsConn.cleanup(bus)
				return

			case evt := <-wsConn.eventChan:
				// Send event to client
				msg := wsConnEventToMessage(evt)
				if err := conn.WriteJSON(msg); err != nil {
					log.Warn("failed to write websocket message", "error", err)
					wsConn.cleanup(bus)
					return
				}

			case <-ticker.C:
				// Send ping to keep connection alive
				if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(10*time.Second)); err != nil {
					log.Warn("failed to send websocket ping", "error", err)
					wsConn.cleanup(bus)
					return
				}
			}
		}
	}
}

// cleanup unsubscribes from all events and closes the event channel.
func (wc *wsConn) cleanup(bus events.Bus) {
	for _, subID := range wc.subscriptionIDs {
		bus.Unsubscribe(subID)
	}
	close(wc.eventChan)
	wc.log.Debug("websocket cleaned up", "subscription_count", len(wc.subscriptionIDs))
}

// wsConnEventToMessage converts an events.Event to a WebSocketMessage.
func wsConnEventToMessage(evt events.Event) WebSocketMessage {
	// Serialize the event to a generic map for JSON encoding
	var payload interface{}

	switch e := evt.(type) {
	case events.RawMemoryCreated:
		payload = e
	case events.MemoryEncoded:
		payload = e
	case events.MemoryAccessed:
		payload = e
	case events.ConsolidationStarted:
		payload = e
	case events.ConsolidationCompleted:
		payload = e
	case events.QueryExecuted:
		payload = e
	case events.MetaCycleCompleted:
		payload = e
	case events.DreamCycleCompleted:
		payload = e
	case events.SystemHealth:
		payload = e
	case events.WatcherEvent:
		payload = e
	default:
		// Fallback for unknown event types
		payload = map[string]interface{}{}
	}

	return WebSocketMessage{
		Type:      evt.EventType(),
		Timestamp: evt.EventTimestamp().UTC().Format(time.RFC3339Nano),
		Payload:   payload,
	}
}
