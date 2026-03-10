// Package web provides a WebSocket channel adapter for browser-based clients.
//
// The adapter manages multiple concurrent WebSocket connections through a
// connection hub. Each connected client is tracked by a unique connection ID.
// Messages are exchanged as JSON using a typed wire protocol.
//
// Wire protocol message types:
//   - "message": A complete text message (inbound or outbound)
//   - "stream_start": Begin a streaming response (outbound only)
//   - "stream_delta": A text chunk in a streaming response (outbound only)
//   - "stream_end": End a streaming response (outbound only)
//
// Configuration uses ChannelConfig.Settings with optional keys:
//   - "path": WebSocket endpoint path (default: "/ws")
//   - "read_buffer_size": Read buffer size in bytes (default: 1024)
//   - "write_buffer_size": Write buffer size in bytes (default: 1024)
//   - "allowed_origins": Comma-separated allowed origins (default: "*")
package web

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"github.com/PhantomMatthew/TianGong/internal/channel"
)

const (
	// SettingPath is the config key for the WebSocket endpoint path.
	SettingPath = "path"
	// SettingReadBufferSize is the config key for read buffer size.
	SettingReadBufferSize = "read_buffer_size"
	// SettingWriteBufferSize is the config key for write buffer size.
	SettingWriteBufferSize = "write_buffer_size"
	// SettingAllowedOrigins is the config key for allowed origins.
	SettingAllowedOrigins = "allowed_origins"

	// DefaultPath is the default WebSocket endpoint path.
	DefaultPath = "/ws"
	// DefaultBufferSize is the default read/write buffer size.
	DefaultBufferSize = 1024

	// writeWait is the time allowed to write a message to a connection.
	writeWait = 10 * time.Second
	// pongWait is the time allowed to read the next pong message.
	pongWait = 60 * time.Second
	// pingPeriod sends pings at this interval. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

// WireMessage is the JSON message exchanged over the WebSocket connection.
type WireMessage struct {
	// Type is the message type: "message", "stream_start", "stream_delta", "stream_end".
	Type string `json:"type"`
	// ID is a unique message identifier.
	ID string `json:"id,omitempty"`
	// Content is the text payload.
	Content string `json:"content,omitempty"`
	// SenderID identifies the sender.
	SenderID string `json:"sender_id,omitempty"`
	// SenderName is the display name of the sender.
	SenderName string `json:"sender_name,omitempty"`
	// ThreadID is the thread/conversation identifier.
	ThreadID string `json:"thread_id,omitempty"`
	// ReplyToID is the message being replied to.
	ReplyToID string `json:"reply_to_id,omitempty"`
	// Metadata holds extra key-value data.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Upgrader abstracts websocket.Upgrader for testability.
type Upgrader interface {
	Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (Conn, error)
}

// Conn abstracts a WebSocket connection for testability.
type Conn interface {
	ReadJSON(v interface{}) error
	WriteJSON(v interface{}) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	SetReadDeadline(t time.Time) error
	SetPongHandler(h func(string) error)
	Close() error
}

// defaultUpgrader wraps gorilla's Upgrader to return the Conn interface.
type defaultUpgrader struct {
	upgrader *websocket.Upgrader
}

func (u *defaultUpgrader) Upgrade(w http.ResponseWriter, r *http.Request, responseHeader http.Header) (Conn, error) {
	conn, err := u.upgrader.Upgrade(w, r, responseHeader)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// connection represents a single WebSocket client connection.
type connection struct {
	id   string
	conn Conn
	mu   sync.Mutex // protects writes
}

// writeJSON sends a JSON message to the connection with write deadline.
func (c *connection) writeJSON(v interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn.WriteJSON(v)
}

// Adapter is a WebSocket channel adapter.
// It manages multiple concurrent client connections through a connection hub.
type Adapter struct {
	name           string
	path           string
	upgrader       Upgrader
	allowedOrigins []string

	mu      sync.RWMutex
	conns   map[string]*connection
	handler channel.InboundHandler
	running bool
	stopCh  chan struct{}
	connSeq atomic.Int64
}

// Option configures the WebSocket adapter.
type Option func(*Adapter)

// WithName sets the adapter instance name (default: "web").
func WithName(name string) Option {
	return func(a *Adapter) {
		a.name = name
	}
}

// WithPath sets the WebSocket endpoint path (default: "/ws").
func WithPath(path string) Option {
	return func(a *Adapter) {
		a.path = path
	}
}

// WithUpgrader sets a custom WebSocket upgrader (primarily for testing).
func WithUpgrader(u Upgrader) Option {
	return func(a *Adapter) {
		a.upgrader = u
	}
}

// WithAllowedOrigins sets the allowed origins for WebSocket connections.
func WithAllowedOrigins(origins []string) Option {
	return func(a *Adapter) {
		a.allowedOrigins = origins
	}
}

// New creates a new WebSocket adapter.
func New(opts ...Option) *Adapter {
	a := &Adapter{
		name:           "web",
		path:           DefaultPath,
		conns:          make(map[string]*connection),
		stopCh:         make(chan struct{}),
		allowedOrigins: []string{"*"},
	}
	for _, opt := range opts {
		opt(a)
	}
	if a.upgrader == nil {
		a.upgrader = &defaultUpgrader{
			upgrader: &websocket.Upgrader{
				ReadBufferSize:  DefaultBufferSize,
				WriteBufferSize: DefaultBufferSize,
				CheckOrigin:     a.checkOrigin,
			},
		}
	}
	return a
}

// NewFromConfig creates a WebSocket adapter from a ChannelConfig.
func NewFromConfig(cfg channel.ChannelConfig, opts ...Option) (*Adapter, error) {
	allOpts := []Option{WithName(cfg.Name)}

	if p, ok := cfg.Settings[SettingPath]; ok && p != "" {
		allOpts = append(allOpts, WithPath(p))
	}
	if origins, ok := cfg.Settings[SettingAllowedOrigins]; ok && origins != "" {
		allOpts = append(allOpts, WithAllowedOrigins(strings.Split(origins, ",")))
	}

	allOpts = append(allOpts, opts...)
	return New(allOpts...), nil
}

// Type returns the channel type identifier.
func (a *Adapter) Type() channel.ChannelType {
	return channel.TypeWeb
}

// Name returns the adapter instance name.
func (a *Adapter) Name() string {
	return a.name
}

// Handler returns the HTTP handler for mounting on a server mux.
// Must be called after Start.
func (a *Adapter) Handler() http.Handler {
	return http.HandlerFunc(a.serveWS)
}

// Path returns the WebSocket endpoint path.
func (a *Adapter) Path() string {
	return a.path
}

// Start begins accepting WebSocket connections. Unlike other adapters,
// the web adapter does not block — it registers itself as ready to handle
// connections and returns immediately. Connections are handled via the
// HTTP handler returned by Handler().
func (a *Adapter) Start(_ context.Context, handler channel.InboundHandler) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return fmt.Errorf("web adapter already running")
	}

	a.handler = handler
	a.running = true

	slog.Info("web adapter started", "name", a.name, "path", a.path)
	return nil
}

// Stop gracefully shuts down all active WebSocket connections.
func (a *Adapter) Stop(_ context.Context) error {
	a.mu.Lock()
	if !a.running {
		a.mu.Unlock()
		return nil
	}
	a.running = false

	// Copy connections to close outside lock.
	toClose := make([]*connection, 0, len(a.conns))
	for _, c := range a.conns {
		toClose = append(toClose, c)
	}
	a.conns = make(map[string]*connection)
	a.mu.Unlock()

	for _, c := range toClose {
		if err := c.conn.Close(); err != nil {
			slog.Warn("error closing websocket connection", "conn_id", c.id, "error", err)
		}
	}

	slog.Info("web adapter stopped", "name", a.name, "connections_closed", len(toClose))
	return nil
}

// Send sends an outbound message to a specific connected client.
// The RecipientID must be a valid connection ID.
func (a *Adapter) Send(_ context.Context, msg channel.OutboundMessage) error {
	conn, err := a.getConn(msg.RecipientID)
	if err != nil {
		return err
	}

	wire := WireMessage{
		Type:      "message",
		ID:        msg.ReplyToID,
		Content:   msg.Content,
		ThreadID:  msg.ThreadID,
		ReplyToID: msg.ReplyToID,
		Metadata:  msg.Metadata,
	}

	if err := conn.writeJSON(wire); err != nil {
		return fmt.Errorf("failed to send websocket message to %s: %w", msg.RecipientID, err)
	}
	return nil
}

// SendStream opens a streaming outbound message to a specific client.
// The RecipientID must be a valid connection ID.
func (a *Adapter) SendStream(_ context.Context, msg channel.OutboundMessage) (channel.OutboundStream, error) {
	conn, err := a.getConn(msg.RecipientID)
	if err != nil {
		return nil, err
	}

	streamID := fmt.Sprintf("stream-%d", a.connSeq.Add(1))

	// Send stream_start.
	start := WireMessage{
		Type:      "stream_start",
		ID:        streamID,
		ThreadID:  msg.ThreadID,
		ReplyToID: msg.ReplyToID,
		Metadata:  msg.Metadata,
	}
	if err := conn.writeJSON(start); err != nil {
		return nil, fmt.Errorf("failed to send stream_start to %s: %w", msg.RecipientID, err)
	}

	return &wsStream{
		conn:     conn,
		streamID: streamID,
	}, nil
}

// ConnCount returns the number of active connections.
func (a *Adapter) ConnCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.conns)
}

// serveWS handles the WebSocket upgrade and manages the connection lifecycle.
func (a *Adapter) serveWS(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	if !a.running {
		a.mu.RUnlock()
		http.Error(w, "adapter not running", http.StatusServiceUnavailable)
		return
	}
	handler := a.handler
	a.mu.RUnlock()

	wsConn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Error("websocket upgrade failed", "error", err, "remote", r.RemoteAddr)
		return
	}

	connID := fmt.Sprintf("ws-%d", a.connSeq.Add(1))
	conn := &connection{
		id:   connID,
		conn: wsConn,
	}

	a.mu.Lock()
	a.conns[connID] = conn
	a.mu.Unlock()

	slog.Info("websocket client connected", "conn_id", connID, "remote", r.RemoteAddr)

	// Set up pong handler for keepalive.
	if err := wsConn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		slog.Error("failed to set read deadline", "conn_id", connID, "error", err)
	}
	wsConn.SetPongHandler(func(_ string) error {
		return wsConn.SetReadDeadline(time.Now().Add(pongWait))
	})

	// Start ping goroutine.
	pingDone := make(chan struct{})
	go a.pingLoop(conn, pingDone)

	// Read messages until connection closes.
	a.readLoop(r.Context(), conn, handler)

	// Cleanup.
	close(pingDone)
	a.removeConn(connID)
	if err := wsConn.Close(); err != nil {
		slog.Debug("websocket close error", "conn_id", connID, "error", err)
	}

	slog.Info("websocket client disconnected", "conn_id", connID)
}

// readLoop reads JSON messages from the connection and dispatches them.
func (a *Adapter) readLoop(ctx context.Context, conn *connection, handler channel.InboundHandler) {
	for {
		var wire WireMessage
		if err := conn.conn.ReadJSON(&wire); err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				slog.Warn("websocket read error", "conn_id", conn.id, "error", err)
			}
			return
		}

		if wire.Content == "" {
			continue
		}

		inbound := channel.InboundMessage{
			ID:          wire.ID,
			ChannelType: channel.TypeWeb,
			ChannelName: a.name,
			SenderID:    wire.SenderID,
			SenderName:  wire.SenderName,
			Content:     wire.Content,
			ThreadID:    wire.ThreadID,
			ReplyToID:   wire.ReplyToID,
			Timestamp:   time.Now(),
			Metadata:    wire.Metadata,
		}

		// Set sender ID to connection ID if not provided.
		if inbound.SenderID == "" {
			inbound.SenderID = conn.id
		}

		// Add connection ID to metadata for routing replies.
		if inbound.Metadata == nil {
			inbound.Metadata = make(map[string]string)
		}
		inbound.Metadata["conn_id"] = conn.id

		if err := handler(ctx, inbound); err != nil {
			slog.Error("websocket handler error",
				"conn_id", conn.id,
				"message_id", wire.ID,
				"error", err,
			)
		}
	}
}

// pingLoop sends periodic pings to keep the connection alive.
func (a *Adapter) pingLoop(conn *connection, done <-chan struct{}) {
	ticker := time.NewTicker(pingPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			conn.mu.Lock()
			err := conn.conn.WriteControl(
				websocket.PingMessage,
				nil,
				time.Now().Add(writeWait),
			)
			conn.mu.Unlock()
			if err != nil {
				slog.Debug("ping failed", "conn_id", conn.id, "error", err)
				return
			}
		}
	}
}

// getConn retrieves a connection by ID.
func (a *Adapter) getConn(id string) (*connection, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.running {
		return nil, fmt.Errorf("web adapter not running")
	}

	conn, ok := a.conns[id]
	if !ok {
		return nil, fmt.Errorf("connection %q not found", id)
	}
	return conn, nil
}

// removeConn removes a connection from the hub.
func (a *Adapter) removeConn(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.conns, id)
}

// checkOrigin validates the request origin against allowed origins.
func (a *Adapter) checkOrigin(r *http.Request) bool {
	for _, o := range a.allowedOrigins {
		if o == "*" {
			return true
		}
		origin := r.Header.Get("Origin")
		if strings.EqualFold(o, origin) {
			return true
		}
	}
	return false
}

// wsStream implements channel.OutboundStream for WebSocket streaming.
type wsStream struct {
	conn     *connection
	streamID string
	closed   bool
	mu       sync.Mutex
}

// Write sends a text delta to the stream.
func (s *wsStream) Write(delta string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("stream %s already closed", s.streamID)
	}

	wire := WireMessage{
		Type:    "stream_delta",
		ID:      s.streamID,
		Content: delta,
	}
	return s.conn.writeJSON(wire)
}

// Close finalizes the streaming message.
func (s *wsStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	wire := WireMessage{
		Type: "stream_end",
		ID:   s.streamID,
	}
	return s.conn.writeJSON(wire)
}
