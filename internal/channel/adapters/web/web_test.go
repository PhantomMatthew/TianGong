package web

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PhantomMatthew/TianGong/internal/channel"
)

// mockConn implements the Conn interface for unit testing.
type mockConn struct {
	mu          sync.Mutex
	written     []interface{}
	readQueue   []interface{} // values to return from ReadJSON
	readIdx     int
	readErr     error // error to return when queue exhausted
	writeErr    error
	controlErr  error
	closed      bool
	pongHandler func(string) error
}

func newMockConn() *mockConn {
	return &mockConn{
		readErr: io.EOF,
	}
}

func (m *mockConn) ReadJSON(v interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.readIdx < len(m.readQueue) {
		src := m.readQueue[m.readIdx]
		m.readIdx++
		// Copy wire message fields.
		if wire, ok := v.(*WireMessage); ok {
			if srcWire, ok := src.(WireMessage); ok {
				*wire = srcWire
				return nil
			}
		}
		return nil
	}
	return m.readErr
}

func (m *mockConn) WriteJSON(v interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.writeErr != nil {
		return m.writeErr
	}
	m.written = append(m.written, v)
	return nil
}

func (m *mockConn) WriteControl(_ int, _ []byte, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.controlErr
}

func (m *mockConn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (m *mockConn) SetPongHandler(h func(string) error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pongHandler = h
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	return nil
}

func (m *mockConn) getWritten() []interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]interface{}, len(m.written))
	copy(cp, m.written)
	return cp
}

func (m *mockConn) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

// mockUpgrader implements the Upgrader interface for testing.
type mockUpgrader struct {
	conn    Conn
	err     error
	mu      sync.Mutex
	calls   int
	connGen func() Conn // generates a new conn per call if set
}

func (u *mockUpgrader) Upgrade(_ http.ResponseWriter, _ *http.Request, _ http.Header) (Conn, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.calls++
	if u.err != nil {
		return nil, u.err
	}
	if u.connGen != nil {
		return u.connGen(), nil
	}
	return u.conn, nil
}

func TestAdapterTypeAndName(t *testing.T) {
	a := New()
	assert.Equal(t, channel.TypeWeb, a.Type())
	assert.Equal(t, "web", a.Name())
}

func TestAdapterCustomName(t *testing.T) {
	a := New(WithName("chat"))
	assert.Equal(t, "chat", a.Name())
}

func TestAdapterDefaultPath(t *testing.T) {
	a := New()
	assert.Equal(t, "/ws", a.Path())
}

func TestAdapterCustomPath(t *testing.T) {
	a := New(WithPath("/api/ws"))
	assert.Equal(t, "/api/ws", a.Path())
}

func TestAdapterImplementsInterfaces(t *testing.T) {
	a := New()
	var _ channel.Adapter = a
	var _ channel.Receiver = a
	var _ channel.Sender = a
	var _ channel.StreamingSender = a
}

func TestNewFromConfig(t *testing.T) {
	tests := []struct {
		name     string
		cfg      channel.ChannelConfig
		wantName string
		wantPath string
		wantErr  bool
	}{
		{
			name: "defaults",
			cfg: channel.ChannelConfig{
				Name:     "webchat",
				Type:     channel.TypeWeb,
				Settings: map[string]string{},
			},
			wantName: "webchat",
			wantPath: DefaultPath,
		},
		{
			name: "custom path",
			cfg: channel.ChannelConfig{
				Name:     "ws",
				Type:     channel.TypeWeb,
				Settings: map[string]string{"path": "/chat/ws"},
			},
			wantName: "ws",
			wantPath: "/chat/ws",
		},
		{
			name: "nil settings",
			cfg: channel.ChannelConfig{
				Name: "web",
				Type: channel.TypeWeb,
			},
			wantName: "web",
			wantPath: DefaultPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewFromConfig(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantName, a.Name())
			assert.Equal(t, tt.wantPath, a.Path())
			assert.Equal(t, channel.TypeWeb, a.Type())
		})
	}
}

func TestStartAndStop(t *testing.T) {
	a := New()

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}

	err := a.Start(context.Background(), handler)
	require.NoError(t, err)

	assert.Equal(t, 0, a.ConnCount())

	err = a.Stop(context.Background())
	require.NoError(t, err)
}

func TestStartAlreadyRunning(t *testing.T) {
	a := New()

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}

	err := a.Start(context.Background(), handler)
	require.NoError(t, err)

	err = a.Start(context.Background(), handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	_ = a.Stop(context.Background())
}

func TestStopWhenNotRunning(t *testing.T) {
	a := New()
	err := a.Stop(context.Background())
	assert.NoError(t, err)
}

func TestStopClosesConnections(t *testing.T) {
	mc := newMockConn()
	a := New()

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	require.NoError(t, a.Start(context.Background(), handler))

	// Manually inject a connection.
	a.mu.Lock()
	a.conns["test-1"] = &connection{id: "test-1", conn: mc}
	a.mu.Unlock()

	assert.Equal(t, 1, a.ConnCount())

	_ = a.Stop(context.Background())

	assert.True(t, mc.isClosed())
	assert.Equal(t, 0, a.ConnCount())
}

func TestSendToConnection(t *testing.T) {
	mc := newMockConn()
	a := New()

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	require.NoError(t, a.Start(context.Background(), handler))

	// Inject a connection.
	a.mu.Lock()
	a.conns["conn-1"] = &connection{id: "conn-1", conn: mc}
	a.mu.Unlock()

	err := a.Send(context.Background(), channel.OutboundMessage{
		Content:     "hello client",
		RecipientID: "conn-1",
		ThreadID:    "thread-42",
	})
	require.NoError(t, err)

	written := mc.getWritten()
	require.Len(t, written, 1)

	wire, ok := written[0].(WireMessage)
	require.True(t, ok)
	assert.Equal(t, "message", wire.Type)
	assert.Equal(t, "hello client", wire.Content)
	assert.Equal(t, "thread-42", wire.ThreadID)

	_ = a.Stop(context.Background())
}

func TestSendToUnknownConnection(t *testing.T) {
	a := New()

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	require.NoError(t, a.Start(context.Background(), handler))

	err := a.Send(context.Background(), channel.OutboundMessage{
		Content:     "hello",
		RecipientID: "nonexistent",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	_ = a.Stop(context.Background())
}

func TestSendBeforeStart(t *testing.T) {
	a := New()

	err := a.Send(context.Background(), channel.OutboundMessage{
		Content:     "hello",
		RecipientID: "conn-1",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not running")
}

func TestSendWriteError(t *testing.T) {
	mc := newMockConn()
	mc.writeErr = fmt.Errorf("connection reset")
	a := New()

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	require.NoError(t, a.Start(context.Background(), handler))

	a.mu.Lock()
	a.conns["conn-1"] = &connection{id: "conn-1", conn: mc}
	a.mu.Unlock()

	err := a.Send(context.Background(), channel.OutboundMessage{
		Content:     "hello",
		RecipientID: "conn-1",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection reset")

	_ = a.Stop(context.Background())
}

func TestSendStream(t *testing.T) {
	mc := newMockConn()
	a := New()

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	require.NoError(t, a.Start(context.Background(), handler))

	a.mu.Lock()
	a.conns["conn-1"] = &connection{id: "conn-1", conn: mc}
	a.mu.Unlock()

	stream, err := a.SendStream(context.Background(), channel.OutboundMessage{
		RecipientID: "conn-1",
		ThreadID:    "t-1",
	})
	require.NoError(t, err)

	// Write deltas.
	require.NoError(t, stream.Write("Hello"))
	require.NoError(t, stream.Write(" world"))

	// Close stream.
	require.NoError(t, stream.Close())

	written := mc.getWritten()
	require.Len(t, written, 4) // stream_start + 2 deltas + stream_end

	// Verify message types.
	assertWireType(t, written[0], "stream_start")
	assertWireType(t, written[1], "stream_delta")
	assertWireType(t, written[2], "stream_delta")
	assertWireType(t, written[3], "stream_end")

	// Verify delta content.
	delta1, ok := written[1].(WireMessage)
	require.True(t, ok)
	assert.Equal(t, "Hello", delta1.Content)

	delta2, ok := written[2].(WireMessage)
	require.True(t, ok)
	assert.Equal(t, " world", delta2.Content)

	_ = a.Stop(context.Background())
}

func TestSendStreamWriteAfterClose(t *testing.T) {
	mc := newMockConn()
	a := New()

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	require.NoError(t, a.Start(context.Background(), handler))

	a.mu.Lock()
	a.conns["conn-1"] = &connection{id: "conn-1", conn: mc}
	a.mu.Unlock()

	stream, err := a.SendStream(context.Background(), channel.OutboundMessage{
		RecipientID: "conn-1",
	})
	require.NoError(t, err)

	require.NoError(t, stream.Close())

	// Write after close should error.
	err = stream.Write("late")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already closed")

	// Double close is a no-op.
	assert.NoError(t, stream.Close())

	_ = a.Stop(context.Background())
}

func TestSendStreamToUnknownConnection(t *testing.T) {
	a := New()

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	require.NoError(t, a.Start(context.Background(), handler))

	_, err := a.SendStream(context.Background(), channel.OutboundMessage{
		RecipientID: "nonexistent",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")

	_ = a.Stop(context.Background())
}

func TestReadLoop(t *testing.T) {
	mc := newMockConn()
	mc.readQueue = []interface{}{
		WireMessage{
			Type:       "message",
			ID:         "msg-1",
			Content:    "hello",
			SenderID:   "user-42",
			SenderName: "Alice",
			ThreadID:   "t-1",
		},
		WireMessage{
			Type:    "message",
			Content: "", // empty content — should be skipped
		},
		WireMessage{
			Type:    "message",
			ID:      "msg-2",
			Content: "world",
		},
	}

	a := New()

	var mu sync.Mutex
	var received []channel.InboundMessage

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}

	conn := &connection{id: "ws-1", conn: mc}
	a.readLoop(context.Background(), conn, handler)

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, received, 2) // empty content skipped

	msg := received[0]
	assert.Equal(t, "msg-1", msg.ID)
	assert.Equal(t, channel.TypeWeb, msg.ChannelType)
	assert.Equal(t, "user-42", msg.SenderID)
	assert.Equal(t, "Alice", msg.SenderName)
	assert.Equal(t, "hello", msg.Content)
	assert.Equal(t, "t-1", msg.ThreadID)
	assert.Equal(t, "ws-1", msg.Metadata["conn_id"])
	assert.False(t, msg.Timestamp.IsZero())

	// Second message has no SenderID — should default to conn ID.
	msg2 := received[1]
	assert.Equal(t, "ws-1", msg2.SenderID)
	assert.Equal(t, "ws-1", msg2.Metadata["conn_id"])
}

func TestServeWSNotRunning(t *testing.T) {
	a := New()

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rec := httptest.NewRecorder()

	a.serveWS(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestServeWSUpgradeError(t *testing.T) {
	upgrader := &mockUpgrader{
		err: fmt.Errorf("upgrade failed"),
	}
	a := New(WithUpgrader(upgrader))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	require.NoError(t, a.Start(context.Background(), handler))

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rec := httptest.NewRecorder()

	a.serveWS(rec, req)

	// Upgrade error is logged, no connection added.
	assert.Equal(t, 0, a.ConnCount())

	_ = a.Stop(context.Background())
}

func TestServeWSFullLifecycle(t *testing.T) {
	mc := newMockConn()
	mc.readQueue = []interface{}{
		WireMessage{
			Type:    "message",
			ID:      "msg-1",
			Content: "hi from browser",
		},
	}

	upgrader := &mockUpgrader{conn: mc}
	a := New(WithUpgrader(upgrader))

	var mu sync.Mutex
	var received []channel.InboundMessage

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}
	require.NoError(t, a.Start(context.Background(), handler))

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rec := httptest.NewRecorder()

	// serveWS blocks until readLoop returns (EOF).
	a.serveWS(rec, req)

	// After serveWS returns, connection is cleaned up.
	assert.Equal(t, 0, a.ConnCount())
	assert.True(t, mc.isClosed())

	mu.Lock()
	require.Len(t, received, 1)
	assert.Equal(t, "hi from browser", received[0].Content)
	assert.Equal(t, channel.TypeWeb, received[0].ChannelType)
	assert.NotEmpty(t, received[0].Metadata["conn_id"])
	mu.Unlock()

	_ = a.Stop(context.Background())
}

func TestCheckOriginWildcard(t *testing.T) {
	a := New() // default: ["*"]

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Origin", "https://evil.com")
	assert.True(t, a.checkOrigin(req))
}

func TestCheckOriginSpecific(t *testing.T) {
	a := New(WithAllowedOrigins([]string{"https://app.tiangong.dev", "https://localhost:3000"}))

	tests := []struct {
		origin string
		want   bool
	}{
		{"https://app.tiangong.dev", true},
		{"https://LOCALHOST:3000", true}, // case insensitive
		{"https://evil.com", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.origin, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/ws", nil)
			req.Header.Set("Origin", tt.origin)
			assert.Equal(t, tt.want, a.checkOrigin(req))
		})
	}
}

func TestConnCount(t *testing.T) {
	a := New()

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	require.NoError(t, a.Start(context.Background(), handler))

	assert.Equal(t, 0, a.ConnCount())

	a.mu.Lock()
	a.conns["c1"] = &connection{id: "c1", conn: newMockConn()}
	a.conns["c2"] = &connection{id: "c2", conn: newMockConn()}
	a.mu.Unlock()

	assert.Equal(t, 2, a.ConnCount())

	_ = a.Stop(context.Background())
}

func TestHandlerReturnsHTTPHandler(t *testing.T) {
	a := New()
	h := a.Handler()
	assert.NotNil(t, h)
	assert.Implements(t, (*http.Handler)(nil), h)
}

// TestIntegrationWebSocket tests the full WebSocket lifecycle using
// a real HTTP test server with gorilla/websocket client.
func TestIntegrationWebSocket(t *testing.T) {
	a := New()

	var mu sync.Mutex
	var received []channel.InboundMessage

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}
	require.NoError(t, a.Start(context.Background(), handler))

	// Create test HTTP server with the adapter's handler.
	server := httptest.NewServer(a.Handler())
	defer server.Close()

	// Connect via WebSocket.
	wsURL := "ws" + server.URL[4:] // http:// -> ws://
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	defer conn.Close()

	// Wait for connection to register.
	time.Sleep(50 * time.Millisecond)
	assert.Equal(t, 1, a.ConnCount())

	// Send a message from client.
	wire := WireMessage{
		Type:       "message",
		ID:         "int-1",
		Content:    "integration test",
		SenderID:   "browser-user",
		SenderName: "TestUser",
	}
	require.NoError(t, conn.WriteJSON(wire))

	// Wait for handler to process.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	assert.Equal(t, "integration test", received[0].Content)
	assert.Equal(t, "browser-user", received[0].SenderID)
	assert.Equal(t, "TestUser", received[0].SenderName)
	assert.Equal(t, channel.TypeWeb, received[0].ChannelType)
	connID := received[0].Metadata["conn_id"]
	assert.NotEmpty(t, connID)
	mu.Unlock()

	// Send a response back to the client via the adapter.
	err = a.Send(context.Background(), channel.OutboundMessage{
		Content:     "hello from server",
		RecipientID: connID,
	})
	require.NoError(t, err)

	// Read the response on the client side.
	var resp WireMessage
	require.NoError(t, conn.ReadJSON(&resp))
	assert.Equal(t, "message", resp.Type)
	assert.Equal(t, "hello from server", resp.Content)

	// Test streaming.
	stream, err := a.SendStream(context.Background(), channel.OutboundMessage{
		RecipientID: connID,
	})
	require.NoError(t, err)

	require.NoError(t, stream.Write("chunk1"))
	require.NoError(t, stream.Write("chunk2"))
	require.NoError(t, stream.Close())

	// Read stream messages on client side.
	var streamStart WireMessage
	require.NoError(t, conn.ReadJSON(&streamStart))
	assert.Equal(t, "stream_start", streamStart.Type)

	var delta1 WireMessage
	require.NoError(t, conn.ReadJSON(&delta1))
	assert.Equal(t, "stream_delta", delta1.Type)
	assert.Equal(t, "chunk1", delta1.Content)

	var delta2 WireMessage
	require.NoError(t, conn.ReadJSON(&delta2))
	assert.Equal(t, "stream_delta", delta2.Type)
	assert.Equal(t, "chunk2", delta2.Content)

	var streamEnd WireMessage
	require.NoError(t, conn.ReadJSON(&streamEnd))
	assert.Equal(t, "stream_end", streamEnd.Type)

	// Close client connection.
	conn.Close()
	time.Sleep(100 * time.Millisecond)

	// Connection should be cleaned up.
	assert.Equal(t, 0, a.ConnCount())

	_ = a.Stop(context.Background())
}

// assertWireType checks that a written value is a WireMessage with the expected type.
func assertWireType(t *testing.T, v interface{}, expectedType string) {
	t.Helper()
	wire, ok := v.(WireMessage)
	require.True(t, ok, "expected WireMessage, got %T", v)
	assert.Equal(t, expectedType, wire.Type)
}
