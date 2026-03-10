package channel

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PhantomMatthew/TianGong/internal/bus"
)

// mockThreadAdapter implements Adapter, Receiver, Sender, TypingIndicator, and ThreadBinder.
type mockThreadAdapter struct {
	mockAdapter
	mu          sync.Mutex
	started     bool
	stopped     bool
	sent        []OutboundMessage
	typingCalls []typingCall
	threadFunc  func(msg InboundMessage) string
	handler     InboundHandler
	stopCh      chan struct{}
}

func newMockThreadAdapter(ct ChannelType, name string) *mockThreadAdapter {
	return &mockThreadAdapter{
		mockAdapter: mockAdapter{channelType: ct, name: name},
		stopCh:      make(chan struct{}),
	}
}

func (m *mockThreadAdapter) Start(ctx context.Context, handler InboundHandler) error {
	m.mu.Lock()
	m.started = true
	m.handler = handler
	m.mu.Unlock()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.stopCh:
		return nil
	}
}

func (m *mockThreadAdapter) Stop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.stopped {
		m.stopped = true
		close(m.stopCh)
	}
	return nil
}

func (m *mockThreadAdapter) Send(_ context.Context, msg OutboundMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func (m *mockThreadAdapter) SendTyping(_ context.Context, recipientID string, action TypingAction) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typingCalls = append(m.typingCalls, typingCall{recipientID: recipientID, action: action})
	return nil
}

func (m *mockThreadAdapter) BindThread(msg InboundMessage) string {
	if m.threadFunc != nil {
		return m.threadFunc(msg)
	}
	return msg.ThreadID
}

// --- ThreadContext tests ---

func TestThreadContextFromMessage(t *testing.T) {
	msg := InboundMessage{
		ThreadID:  "thread-42",
		ReplyToID: "msg-99",
	}
	tc := ThreadContextFromMessage(msg)
	assert.Equal(t, "thread-42", tc.ThreadID)
	assert.Equal(t, "msg-99", tc.ParentMessageID)
	assert.True(t, tc.IsThread)
}

func TestThreadContextFromMessageNoThread(t *testing.T) {
	msg := InboundMessage{
		SenderID: "user-1",
	}
	tc := ThreadContextFromMessage(msg)
	assert.Empty(t, tc.ThreadID)
	assert.Empty(t, tc.ParentMessageID)
	assert.False(t, tc.IsThread)
}

// --- ThreadAwareSessionResolver tests ---

func TestThreadAwareSessionResolverWithThread(t *testing.T) {
	msg := InboundMessage{
		ChannelType: TypeTelegram,
		SenderID:    "user-1",
		ThreadID:    "chat-42",
	}
	sessionID := ThreadAwareSessionResolver(msg)
	assert.Equal(t, "telegram:chat-42:user-1", sessionID)
}

func TestThreadAwareSessionResolverWithoutThread(t *testing.T) {
	msg := InboundMessage{
		ChannelType: TypeTelegram,
		SenderID:    "user-1",
	}
	sessionID := ThreadAwareSessionResolver(msg)
	assert.Equal(t, "telegram:user-1", sessionID)
}

func TestThreadAwareSessionResolverDifferentChannels(t *testing.T) {
	tests := []struct {
		channelType ChannelType
		senderID    string
		threadID    string
		expected    string
	}{
		{TypeDiscord, "user-1", "thread-abc", "discord:thread-abc:user-1"},
		{TypeSlack, "U123", "", "slack:U123"},
		{TypeCLI, "local", "", "cli:local"},
		{TypeWeb, "ws-1", "room-5", "web:room-5:ws-1"},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%s_%s", tt.channelType, tt.threadID), func(t *testing.T) {
			msg := InboundMessage{
				ChannelType: tt.channelType,
				SenderID:    tt.senderID,
				ThreadID:    tt.threadID,
			}
			assert.Equal(t, tt.expected, ThreadAwareSessionResolver(msg))
		})
	}
}

// --- Registry ThreadBinder tests ---

func TestRegistryDetectsThreadBinder(t *testing.T) {
	reg := NewRegistry()
	adapter := newMockThreadAdapter(TypeTelegram, "tg-1")
	cfg := ChannelConfig{Type: TypeTelegram, Name: "tg-1", Enabled: true}

	err := reg.Register(adapter, cfg)
	require.NoError(t, err)

	tb, ok := reg.GetThreadBinder("tg-1")
	assert.True(t, ok)
	assert.NotNil(t, tb)
}

func TestRegistryNoThreadBinder(t *testing.T) {
	reg := NewRegistry()
	adapter := &mockAdapter{channelType: TypeCLI, name: "cli"}
	cfg := ChannelConfig{Type: TypeCLI, Name: "cli", Enabled: true}

	err := reg.Register(adapter, cfg)
	require.NoError(t, err)

	tb, ok := reg.GetThreadBinder("cli")
	assert.False(t, ok)
	assert.Nil(t, tb)
}

func TestRegistryGetThreadBinderNotFound(t *testing.T) {
	reg := NewRegistry()
	tb, ok := reg.GetThreadBinder("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, tb)
}

// --- Router thread binding tests ---

func TestRouterUsesThreadBinder(t *testing.T) {
	reg := NewRegistry()
	eb := bus.New()
	defer eb.Close()

	adapter := newMockThreadAdapter(TypeTelegram, "tg-1")
	// Custom thread binding: always returns "bound-thread-42".
	adapter.threadFunc = func(_ InboundMessage) string {
		return "bound-thread-42"
	}
	cfg := ChannelConfig{Type: TypeTelegram, Name: "tg-1", Enabled: true}
	require.NoError(t, reg.Register(adapter, cfg))

	router, err := NewRouter(reg, eb, RouterConfig{
		Handler: func(_ context.Context, _ string, content string) (string, error) {
			return "reply: " + content, nil
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, router.Start(ctx))

	// Wait for receiver to start.
	time.Sleep(50 * time.Millisecond)

	// Simulate inbound message.
	adapter.mu.Lock()
	h := adapter.handler
	adapter.mu.Unlock()
	require.NotNil(t, h)

	inbound := InboundMessage{
		ID:          "msg-1",
		ChannelType: TypeTelegram,
		ChannelName: "tg-1",
		SenderID:    "user-1",
		Content:     "hello",
		ThreadID:    "original-thread",
	}
	require.NoError(t, h(ctx, inbound))

	// Wait for async processing.
	time.Sleep(200 * time.Millisecond)

	adapter.mu.Lock()
	defer adapter.mu.Unlock()

	// ThreadBinder should override the thread ID.
	require.Len(t, adapter.sent, 1)
	assert.Equal(t, "bound-thread-42", adapter.sent[0].ThreadID)
	assert.Equal(t, "reply: hello", adapter.sent[0].Content)
}

func TestRouterFallsBackToMessageThreadID(t *testing.T) {
	reg := NewRegistry()
	eb := bus.New()
	defer eb.Close()

	// Use a plain adapter without ThreadBinder.
	adapter := newMockFullAdapter(TypeCLI, "cli-1")
	cfg := ChannelConfig{Type: TypeCLI, Name: "cli-1", Enabled: true}
	require.NoError(t, reg.Register(adapter, cfg))

	router, err := NewRouter(reg, eb, RouterConfig{
		Handler: func(_ context.Context, _ string, _ string) (string, error) {
			return "ok", nil
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, router.Start(ctx))
	time.Sleep(50 * time.Millisecond)

	adapter.mu.Lock()
	h := adapter.handler
	adapter.mu.Unlock()
	require.NotNil(t, h)

	inbound := InboundMessage{
		ID:          "msg-1",
		ChannelType: TypeCLI,
		ChannelName: "cli-1",
		SenderID:    "local",
		Content:     "test",
		ThreadID:    "thread-from-message",
	}
	require.NoError(t, h(ctx, inbound))
	time.Sleep(200 * time.Millisecond)

	adapter.mu.Lock()
	defer adapter.mu.Unlock()

	// Without ThreadBinder, original ThreadID is preserved.
	require.Len(t, adapter.sent, 1)
	assert.Equal(t, "thread-from-message", adapter.sent[0].ThreadID)
}

// --- Router typing indicator tests ---

func TestRouterSendsTypingBeforeHandler(t *testing.T) {
	reg := NewRegistry()
	eb := bus.New()
	defer eb.Close()

	adapter := newMockThreadAdapter(TypeTelegram, "tg-1")
	cfg := ChannelConfig{Type: TypeTelegram, Name: "tg-1", Enabled: true}
	require.NoError(t, reg.Register(adapter, cfg))

	var handlerCalled atomic.Bool
	router, err := NewRouter(reg, eb, RouterConfig{
		Handler: func(_ context.Context, _ string, _ string) (string, error) {
			handlerCalled.Store(true)
			return "response", nil
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, router.Start(ctx))
	time.Sleep(50 * time.Millisecond)

	adapter.mu.Lock()
	h := adapter.handler
	adapter.mu.Unlock()
	require.NotNil(t, h)

	inbound := InboundMessage{
		ID:          "msg-1",
		ChannelType: TypeTelegram,
		ChannelName: "tg-1",
		SenderID:    "user-1",
		Content:     "hello",
	}
	require.NoError(t, h(ctx, inbound))
	time.Sleep(200 * time.Millisecond)

	assert.True(t, handlerCalled.Load())

	adapter.mu.Lock()
	defer adapter.mu.Unlock()

	// Typing should have been sent to the sender.
	require.Len(t, adapter.typingCalls, 1)
	assert.Equal(t, "user-1", adapter.typingCalls[0].recipientID)
	assert.Equal(t, TypingActionTyping, adapter.typingCalls[0].action)
}

func TestRouterSendsTypingToThreadInGroupChat(t *testing.T) {
	reg := NewRegistry()
	eb := bus.New()
	defer eb.Close()

	adapter := newMockThreadAdapter(TypeTelegram, "tg-1")
	cfg := ChannelConfig{Type: TypeTelegram, Name: "tg-1", Enabled: true}
	require.NoError(t, reg.Register(adapter, cfg))

	router, err := NewRouter(reg, eb, RouterConfig{
		Handler: func(_ context.Context, _ string, _ string) (string, error) {
			return "ok", nil
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, router.Start(ctx))
	time.Sleep(50 * time.Millisecond)

	adapter.mu.Lock()
	h := adapter.handler
	adapter.mu.Unlock()
	require.NotNil(t, h)

	// Simulate group chat message with thread.
	inbound := InboundMessage{
		ID:          "msg-1",
		ChannelType: TypeTelegram,
		ChannelName: "tg-1",
		SenderID:    "user-1",
		Content:     "hello",
		ThreadID:    "group-chat-42",
	}
	require.NoError(t, h(ctx, inbound))
	time.Sleep(200 * time.Millisecond)

	adapter.mu.Lock()
	defer adapter.mu.Unlock()

	// Typing should be sent to the thread (group chat), not the user.
	require.Len(t, adapter.typingCalls, 1)
	assert.Equal(t, "group-chat-42", adapter.typingCalls[0].recipientID)
}

func TestRouterNoTypingForAdapterWithoutInterface(t *testing.T) {
	reg := NewRegistry()
	eb := bus.New()
	defer eb.Close()

	// mockFullAdapter does NOT implement TypingIndicator.
	adapter := newMockFullAdapter(TypeCLI, "cli-1")
	cfg := ChannelConfig{Type: TypeCLI, Name: "cli-1", Enabled: true}
	require.NoError(t, reg.Register(adapter, cfg))

	router, err := NewRouter(reg, eb, RouterConfig{
		Handler: func(_ context.Context, _ string, _ string) (string, error) {
			return "ok", nil
		},
	})
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, router.Start(ctx))
	time.Sleep(50 * time.Millisecond)

	adapter.mu.Lock()
	h := adapter.handler
	adapter.mu.Unlock()
	require.NotNil(t, h)

	inbound := InboundMessage{
		ID:          "msg-1",
		ChannelType: TypeCLI,
		ChannelName: "cli-1",
		SenderID:    "local",
		Content:     "test",
	}
	require.NoError(t, h(ctx, inbound))
	time.Sleep(200 * time.Millisecond)

	// No panic, message still sent.
	adapter.mu.Lock()
	defer adapter.mu.Unlock()
	require.Len(t, adapter.sent, 1)
	assert.Equal(t, "ok", adapter.sent[0].Content)
}
