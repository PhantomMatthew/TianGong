package channel

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAdapter implements Adapter only (no Receiver or Sender).
type mockAdapter struct {
	channelType ChannelType
	name        string
}

func (m *mockAdapter) Type() ChannelType { return m.channelType }
func (m *mockAdapter) Name() string      { return m.name }

// mockFullAdapter implements Adapter, Receiver, and Sender.
type mockFullAdapter struct {
	mockAdapter
	mu      sync.Mutex
	started bool
	stopped bool
	sendErr error
	sent    []OutboundMessage
	handler InboundHandler
	stopCh  chan struct{}
}

func newMockFullAdapter(ct ChannelType, name string) *mockFullAdapter {
	return &mockFullAdapter{
		mockAdapter: mockAdapter{channelType: ct, name: name},
		stopCh:      make(chan struct{}),
	}
}

func (m *mockFullAdapter) Start(ctx context.Context, handler InboundHandler) error {
	m.mu.Lock()
	m.started = true
	m.handler = handler
	m.mu.Unlock()
	// Block until stopped or context cancelled.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.stopCh:
		return nil
	}
}

func (m *mockFullAdapter) Stop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.stopped {
		m.stopped = true
		close(m.stopCh)
	}
	return nil
}

func (m *mockFullAdapter) Send(_ context.Context, msg OutboundMessage) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sent = append(m.sent, msg)
	return nil
}

func TestRegisterAndGet(t *testing.T) {
	reg := NewRegistry()
	adapter := &mockAdapter{channelType: TypeCLI, name: "test-cli"}
	cfg := ChannelConfig{Type: TypeCLI, Name: "test-cli", Enabled: true}

	err := reg.Register(adapter, cfg)
	require.NoError(t, err)

	got, ok := reg.Get("test-cli")
	assert.True(t, ok)
	assert.Equal(t, adapter, got)
}

func TestRegisterUsesAdapterTypeAsDefaultName(t *testing.T) {
	reg := NewRegistry()
	adapter := &mockAdapter{channelType: TypeTelegram, name: "tg"}
	cfg := ChannelConfig{Type: TypeTelegram, Name: "", Enabled: true}

	err := reg.Register(adapter, cfg)
	require.NoError(t, err)

	got, ok := reg.Get("telegram")
	assert.True(t, ok)
	assert.Equal(t, TypeTelegram, got.Type())
}

func TestRegisterDuplicateReturnsError(t *testing.T) {
	reg := NewRegistry()
	adapter := &mockAdapter{channelType: TypeCLI, name: "cli"}
	cfg := ChannelConfig{Type: TypeCLI, Name: "cli", Enabled: true}

	require.NoError(t, reg.Register(adapter, cfg))
	err := reg.Register(adapter, cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already registered")
}

func TestGetNonExistent(t *testing.T) {
	reg := NewRegistry()

	_, ok := reg.Get("nonexistent")
	assert.False(t, ok)
}

func TestGetSenderWithSender(t *testing.T) {
	reg := NewRegistry()
	adapter := newMockFullAdapter(TypeCLI, "cli-sender")
	cfg := ChannelConfig{Type: TypeCLI, Name: "cli-sender", Enabled: true}

	require.NoError(t, reg.Register(adapter, cfg))

	sender, ok := reg.GetSender("cli-sender")
	assert.True(t, ok)
	assert.NotNil(t, sender)
}

func TestGetSenderWithoutSender(t *testing.T) {
	reg := NewRegistry()
	adapter := &mockAdapter{channelType: TypeCLI, name: "nosend"}
	cfg := ChannelConfig{Type: TypeCLI, Name: "nosend", Enabled: true}

	require.NoError(t, reg.Register(adapter, cfg))

	sender, ok := reg.GetSender("nosend")
	assert.False(t, ok)
	assert.Nil(t, sender)
}

func TestGetSenderNonExistent(t *testing.T) {
	reg := NewRegistry()

	sender, ok := reg.GetSender("nope")
	assert.False(t, ok)
	assert.Nil(t, sender)
}

func TestList(t *testing.T) {
	reg := NewRegistry()

	cfgA := ChannelConfig{Type: TypeCLI, Name: "a", Enabled: true}
	cfgB := ChannelConfig{Type: TypeTelegram, Name: "b", Enabled: false}

	require.NoError(t, reg.Register(&mockAdapter{channelType: TypeCLI, name: "a"}, cfgA))
	require.NoError(t, reg.Register(&mockAdapter{channelType: TypeTelegram, name: "b"}, cfgB))

	list := reg.List()
	assert.Len(t, list, 2)

	names := make(map[string]bool)
	for _, c := range list {
		names[c.Name] = true
	}
	assert.True(t, names["a"])
	assert.True(t, names["b"])
}

func TestListEmpty(t *testing.T) {
	reg := NewRegistry()
	assert.Empty(t, reg.List())
}

func TestStartAllStartsEnabledReceivers(t *testing.T) {
	reg := NewRegistry()
	adapter := newMockFullAdapter(TypeCLI, "live")
	cfg := ChannelConfig{Type: TypeCLI, Name: "live", Enabled: true}
	require.NoError(t, reg.Register(adapter, cfg))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := func(_ context.Context, _ InboundMessage) error { return nil }
	err := reg.StartAll(ctx, handler)
	require.NoError(t, err)

	// Give goroutine time to start.
	time.Sleep(50 * time.Millisecond)

	adapter.mu.Lock()
	started := adapter.started
	adapter.mu.Unlock()
	assert.True(t, started)

	// Cleanup
	require.NoError(t, reg.StopAll(ctx))
}

func TestStartAllSkipsDisabledReceivers(t *testing.T) {
	reg := NewRegistry()
	adapter := newMockFullAdapter(TypeCLI, "disabled")
	cfg := ChannelConfig{Type: TypeCLI, Name: "disabled", Enabled: false}
	require.NoError(t, reg.Register(adapter, cfg))

	ctx := context.Background()
	handler := func(_ context.Context, _ InboundMessage) error { return nil }
	err := reg.StartAll(ctx, handler)
	require.NoError(t, err)

	// Give goroutines a chance to run (they shouldn't).
	time.Sleep(50 * time.Millisecond)

	adapter.mu.Lock()
	started := adapter.started
	adapter.mu.Unlock()
	assert.False(t, started)
}

func TestStartAllSkipsAdapterOnlyAdapters(t *testing.T) {
	reg := NewRegistry()
	adapter := &mockAdapter{channelType: TypeCLI, name: "basic"}
	cfg := ChannelConfig{Type: TypeCLI, Name: "basic", Enabled: true}
	require.NoError(t, reg.Register(adapter, cfg))

	ctx := context.Background()
	handler := func(_ context.Context, _ InboundMessage) error { return nil }
	err := reg.StartAll(ctx, handler)
	require.NoError(t, err)
}

func TestStartAllDoubleStartReturnsError(t *testing.T) {
	reg := NewRegistry()
	ctx := context.Background()
	handler := func(_ context.Context, _ InboundMessage) error { return nil }

	require.NoError(t, reg.StartAll(ctx, handler))
	err := reg.StartAll(ctx, handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")
}

func TestStopAllStopsReceivers(t *testing.T) {
	reg := NewRegistry()
	adapter := newMockFullAdapter(TypeCLI, "stoppable")
	cfg := ChannelConfig{Type: TypeCLI, Name: "stoppable", Enabled: true}
	require.NoError(t, reg.Register(adapter, cfg))

	ctx := context.Background()
	handler := func(_ context.Context, _ InboundMessage) error { return nil }
	require.NoError(t, reg.StartAll(ctx, handler))

	time.Sleep(50 * time.Millisecond)

	err := reg.StopAll(ctx)
	require.NoError(t, err)

	adapter.mu.Lock()
	stopped := adapter.stopped
	adapter.mu.Unlock()
	assert.True(t, stopped)
}

func TestStopAllWhenNotRunningIsNoop(t *testing.T) {
	reg := NewRegistry()
	err := reg.StopAll(context.Background())
	assert.NoError(t, err)
}
