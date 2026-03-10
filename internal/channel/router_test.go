package channel

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PhantomMatthew/TianGong/internal/bus"
)

func TestNewRouterRequiresHandler(t *testing.T) {
	reg := NewRegistry()
	eb := bus.New()
	defer eb.Close()

	_, err := NewRouter(reg, eb, RouterConfig{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "handler is required")
}

func TestNewRouterWithDefaults(t *testing.T) {
	reg := NewRegistry()
	eb := bus.New()
	defer eb.Close()

	handler := func(_ context.Context, _ string, _ string) (string, error) {
		return "ok", nil
	}

	router, err := NewRouter(reg, eb, RouterConfig{Handler: handler})
	require.NoError(t, err)
	assert.NotNil(t, router)
}

func TestNewRouterWithCustomSessionResolver(t *testing.T) {
	reg := NewRegistry()
	eb := bus.New()
	defer eb.Close()

	customResolver := func(msg InboundMessage) string {
		return fmt.Sprintf("custom:%s", msg.SenderID)
	}

	router, err := NewRouter(reg, eb, RouterConfig{
		Handler:         func(_ context.Context, _ string, _ string) (string, error) { return "", nil },
		SessionResolver: customResolver,
	})
	require.NoError(t, err)
	assert.NotNil(t, router)
}

func TestDefaultSessionResolverFormat(t *testing.T) {
	msg := InboundMessage{
		ChannelType: TypeTelegram,
		SenderID:    "user123",
	}
	sessionID := DefaultSessionResolver(msg)
	assert.Equal(t, "telegram:user123", sessionID)
}

func TestDefaultSessionResolverDifferentChannels(t *testing.T) {
	tests := []struct {
		channelType ChannelType
		senderID    string
		expected    string
	}{
		{TypeCLI, "local", "cli:local"},
		{TypeDiscord, "disc456", "discord:disc456"},
		{TypeSlack, "U123ABC", "slack:U123ABC"},
	}

	for _, tt := range tests {
		t.Run(string(tt.channelType), func(t *testing.T) {
			msg := InboundMessage{ChannelType: tt.channelType, SenderID: tt.senderID}
			assert.Equal(t, tt.expected, DefaultSessionResolver(msg))
		})
	}
}

func TestRouterStartAndStop(t *testing.T) {
	reg := NewRegistry()
	eb := bus.New()
	defer eb.Close()

	adapter := newMockFullAdapter(TypeCLI, "cli")
	cfg := ChannelConfig{Type: TypeCLI, Name: "cli", Enabled: true}
	require.NoError(t, reg.Register(adapter, cfg))

	router, err := NewRouter(reg, eb, RouterConfig{
		Handler: func(_ context.Context, _ string, _ string) (string, error) { return "reply", nil },
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = router.Start(ctx)
	require.NoError(t, err)

	time.Sleep(50 * time.Millisecond)

	adapter.mu.Lock()
	started := adapter.started
	adapter.mu.Unlock()
	assert.True(t, started, "receiver should have been started")

	err = router.Stop(ctx)
	require.NoError(t, err)

	adapter.mu.Lock()
	stopped := adapter.stopped
	adapter.mu.Unlock()
	assert.True(t, stopped, "receiver should have been stopped")
}

func TestRouterDoubleStartReturnsError(t *testing.T) {
	reg := NewRegistry()
	eb := bus.New()
	defer eb.Close()

	router, err := NewRouter(reg, eb, RouterConfig{
		Handler: func(_ context.Context, _ string, _ string) (string, error) { return "", nil },
	})
	require.NoError(t, err)

	ctx := context.Background()
	require.NoError(t, router.Start(ctx))

	err = router.Start(ctx)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

func TestRouterStopWhenNotRunningIsNoop(t *testing.T) {
	reg := NewRegistry()
	eb := bus.New()
	defer eb.Close()

	router, err := NewRouter(reg, eb, RouterConfig{
		Handler: func(_ context.Context, _ string, _ string) (string, error) { return "", nil },
	})
	require.NoError(t, err)

	err = router.Stop(context.Background())
	assert.NoError(t, err)
}

func TestRouterEndToEnd(t *testing.T) {
	reg := NewRegistry()
	eb := bus.New()
	defer eb.Close()

	adapter := newMockFullAdapter(TypeCLI, "e2e-cli")
	cfg := ChannelConfig{Type: TypeCLI, Name: "e2e-cli", Enabled: true}
	require.NoError(t, reg.Register(adapter, cfg))

	// Subscribe to bus events before router starts.
	recvSub := eb.Subscribe(bus.EventMessageReceived)
	sentSub := eb.Subscribe(bus.EventMessageSent)

	var handlerMu sync.Mutex
	handlerCalls := make(map[string]string) // sessionID -> content
	router, err := NewRouter(reg, eb, RouterConfig{
		Handler: func(_ context.Context, sessionID string, content string) (string, error) {
			handlerMu.Lock()
			handlerCalls[sessionID] = content
			handlerMu.Unlock()
			return "echo: " + content, nil
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, router.Start(ctx))

	// Wait for receiver to start.
	time.Sleep(50 * time.Millisecond)

	// Grab the handler the router passed to the receiver.
	adapter.mu.Lock()
	handler := adapter.handler
	adapter.mu.Unlock()
	require.NotNil(t, handler, "receiver should have a handler after Start")

	// Simulate an inbound message via the handler.
	inbound := InboundMessage{
		ID:          "msg-1",
		ChannelType: TypeCLI,
		ChannelName: "e2e-cli",
		SenderID:    "user-42",
		SenderName:  "Test User",
		Content:     "hello tiangong",
		Timestamp:   time.Now(),
	}
	err = handler(ctx, inbound)
	require.NoError(t, err)

	// Wait for async processing.
	time.Sleep(200 * time.Millisecond)

	// Verify handler was called with correct session ID.
	handlerMu.Lock()
	assert.Equal(t, "hello tiangong", handlerCalls["cli:user-42"])
	handlerMu.Unlock()

	// Verify response was sent.
	adapter.mu.Lock()
	require.Len(t, adapter.sent, 1)
	assert.Equal(t, "echo: hello tiangong", adapter.sent[0].Content)
	assert.Equal(t, "user-42", adapter.sent[0].RecipientID)
	assert.Equal(t, TypeCLI, adapter.sent[0].ChannelType)
	assert.Equal(t, "e2e-cli", adapter.sent[0].ChannelName)
	assert.Equal(t, "msg-1", adapter.sent[0].ReplyToID)
	adapter.mu.Unlock()

	// Verify bus events were published.
	select {
	case evt := <-recvSub.C():
		assert.Equal(t, bus.EventMessageReceived, evt.Type)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message.received event")
	}

	select {
	case evt := <-sentSub.C():
		assert.Equal(t, bus.EventMessageSent, evt.Type)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message.sent event")
	}

	// Cleanup.
	require.NoError(t, router.Stop(ctx))
}

func TestRouterHandlerErrorSendsErrorResponse(t *testing.T) {
	reg := NewRegistry()
	eb := bus.New()
	defer eb.Close()

	adapter := newMockFullAdapter(TypeCLI, "err-cli")
	cfg := ChannelConfig{Type: TypeCLI, Name: "err-cli", Enabled: true}
	require.NoError(t, reg.Register(adapter, cfg))

	router, err := NewRouter(reg, eb, RouterConfig{
		Handler: func(_ context.Context, _ string, _ string) (string, error) {
			return "", fmt.Errorf("agent exploded")
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, router.Start(ctx))
	time.Sleep(50 * time.Millisecond)

	adapter.mu.Lock()
	handler := adapter.handler
	adapter.mu.Unlock()

	err = handler(ctx, InboundMessage{
		ID:          "msg-err",
		ChannelType: TypeCLI,
		ChannelName: "err-cli",
		SenderID:    "user-99",
		Content:     "break things",
		Timestamp:   time.Now(),
	})
	require.NoError(t, err) // handleInbound itself returns nil (processes async)

	time.Sleep(200 * time.Millisecond)

	// Verify fallback error response was sent.
	adapter.mu.Lock()
	require.Len(t, adapter.sent, 1)
	assert.Contains(t, adapter.sent[0].Content, "error processing your message")
	adapter.mu.Unlock()

	require.NoError(t, router.Stop(ctx))
}

func TestRouterNilEventBus(t *testing.T) {
	reg := NewRegistry()
	adapter := newMockFullAdapter(TypeCLI, "no-bus")
	cfg := ChannelConfig{Type: TypeCLI, Name: "no-bus", Enabled: true}
	require.NoError(t, reg.Register(adapter, cfg))

	router, err := NewRouter(reg, nil, RouterConfig{
		Handler: func(_ context.Context, _ string, _ string) (string, error) { return "ok", nil },
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, router.Start(ctx))
	time.Sleep(50 * time.Millisecond)

	adapter.mu.Lock()
	handler := adapter.handler
	adapter.mu.Unlock()

	// Should not panic with nil event bus.
	err = handler(ctx, InboundMessage{
		ID:          "msg-nobus",
		ChannelType: TypeCLI,
		ChannelName: "no-bus",
		SenderID:    "user-1",
		Content:     "hi",
		Timestamp:   time.Now(),
	})
	require.NoError(t, err)

	time.Sleep(200 * time.Millisecond)

	adapter.mu.Lock()
	require.Len(t, adapter.sent, 1)
	assert.Equal(t, "ok", adapter.sent[0].Content)
	adapter.mu.Unlock()

	require.NoError(t, router.Stop(ctx))
}
