package discord

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PhantomMatthew/TianGong/internal/channel"
)

// mockSession implements the Session interface for testing.
type mockSession struct {
	mu sync.Mutex

	openCalled  bool
	closeCalled bool
	openErr     error
	closeErr    error

	handlers    []interface{}
	typingCalls []string
	typingErr   error

	sendCalls    []sendCall
	complexCalls []complexCall
	sendErr      error
	complexErr   error

	channels   map[string]*discordgo.Channel
	channelErr error
}

type sendCall struct {
	channelID string
	content   string
}

type complexCall struct {
	channelID string
	data      *discordgo.MessageSend
}

func newMockSession() *mockSession {
	return &mockSession{
		channels: make(map[string]*discordgo.Channel),
	}
}

func (m *mockSession) Open() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.openCalled = true
	return m.openErr
}

func (m *mockSession) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closeCalled = true
	return m.closeErr
}

func (m *mockSession) AddHandler(handler interface{}) func() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers = append(m.handlers, handler)
	return func() {}
}

func (m *mockSession) ChannelMessageSend(channelID string, content string, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendCalls = append(m.sendCalls, sendCall{channelID: channelID, content: content})
	if m.sendErr != nil {
		return nil, m.sendErr
	}
	return &discordgo.Message{ID: "sent-1"}, nil
}

func (m *mockSession) ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, _ ...discordgo.RequestOption) (*discordgo.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.complexCalls = append(m.complexCalls, complexCall{channelID: channelID, data: data})
	if m.complexErr != nil {
		return nil, m.complexErr
	}
	return &discordgo.Message{ID: "sent-2"}, nil
}

func (m *mockSession) ChannelTyping(channelID string, _ ...discordgo.RequestOption) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.typingCalls = append(m.typingCalls, channelID)
	return m.typingErr
}

func (m *mockSession) Channel(channelID string, _ ...discordgo.RequestOption) (*discordgo.Channel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.channelErr != nil {
		return nil, m.channelErr
	}
	ch, ok := m.channels[channelID]
	if !ok {
		return &discordgo.Channel{ID: channelID, Type: discordgo.ChannelTypeGuildText}, nil
	}
	return ch, nil
}

// getHandlers returns a copy of registered handlers.
func (m *mockSession) getHandlers() []interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]interface{}, len(m.handlers))
	copy(cp, m.handlers)
	return cp
}

// getSendCalls returns a copy of send calls.
func (m *mockSession) getSendCalls() []sendCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]sendCall, len(m.sendCalls))
	copy(cp, m.sendCalls)
	return cp
}

// getComplexCalls returns a copy of complex send calls.
func (m *mockSession) getComplexCalls() []complexCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]complexCall, len(m.complexCalls))
	copy(cp, m.complexCalls)
	return cp
}

// getTypingCalls returns a copy of typing calls.
func (m *mockSession) getTypingCalls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]string, len(m.typingCalls))
	copy(cp, m.typingCalls)
	return cp
}

// mockSessionFactory returns a SessionFactory that injects the given mock.
func mockSessionFactory(s Session) SessionFactory {
	return func(_ string) (Session, error) {
		return s, nil
	}
}

// failingSessionFactory returns a SessionFactory that always fails.
func failingSessionFactory() SessionFactory {
	return func(_ string) (Session, error) {
		return nil, fmt.Errorf("connection refused")
	}
}

// invokeMessageHandler finds the registered MessageCreate handler and invokes it.
func invokeMessageHandler(m *mockSession, msg *discordgo.MessageCreate) {
	handlers := m.getHandlers()
	for _, h := range handlers {
		if fn, ok := h.(func(*discordgo.Session, *discordgo.MessageCreate)); ok {
			fn(nil, msg)
		}
	}
}

// --- Type & Name Tests ---

func TestAdapterTypeAndName(t *testing.T) {
	a := New("test-token")
	assert.Equal(t, channel.TypeDiscord, a.Type())
	assert.Equal(t, "discord", a.Name())
}

func TestAdapterCustomName(t *testing.T) {
	a := New("test-token", WithName("my-discord-bot"))
	assert.Equal(t, "my-discord-bot", a.Name())
}

func TestAdapterImplementsInterfaces(t *testing.T) {
	a := New("test-token")
	var _ channel.Adapter = a
	var _ channel.Receiver = a
	var _ channel.Sender = a
	var _ channel.TypingIndicator = a
	var _ channel.ThreadBinder = a
}

// --- NewFromConfig Tests ---

func TestNewFromConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     channel.ChannelConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: channel.ChannelConfig{
				Name:     "prod-discord",
				Type:     channel.TypeDiscord,
				Settings: map[string]string{"token": "abc123"},
			},
			wantErr: false,
		},
		{
			name: "missing token",
			cfg: channel.ChannelConfig{
				Name:     "no-token",
				Type:     channel.TypeDiscord,
				Settings: map[string]string{},
			},
			wantErr: true,
		},
		{
			name: "empty token",
			cfg: channel.ChannelConfig{
				Name:     "empty-token",
				Type:     channel.TypeDiscord,
				Settings: map[string]string{"token": ""},
			},
			wantErr: true,
		},
		{
			name: "nil settings",
			cfg: channel.ChannelConfig{
				Name: "nil-settings",
				Type: channel.TypeDiscord,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, err := NewFromConfig(tt.cfg)
			if tt.wantErr {
				assert.Error(t, err)
				assert.Nil(t, a)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.cfg.Name, a.Name())
				assert.Equal(t, channel.TypeDiscord, a.Type())
			}
		})
	}
}

// --- Start/Stop Tests ---

func TestStartAndStop(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Start(context.Background(), handler)
	}()

	// Give Start time to open session and register handler.
	time.Sleep(50 * time.Millisecond)

	err := a.Stop(context.Background())
	require.NoError(t, err)

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop")
	}

	mock.mu.Lock()
	assert.True(t, mock.openCalled)
	mock.mu.Unlock()
}

func TestStartStopsOnContextCancel(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Start(ctx, handler)
	}()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}

func TestStartAlreadyRunning(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}

	go func() {
		_ = a.Start(context.Background(), handler)
	}()

	time.Sleep(50 * time.Millisecond)

	err := a.Start(context.Background(), handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	_ = a.Stop(context.Background())
}

func TestStartSessionFactoryError(t *testing.T) {
	a := New("test-token", WithSessionFactory(failingSessionFactory()))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}

	err := a.Start(context.Background(), handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestStartOpenError(t *testing.T) {
	mock := newMockSession()
	mock.openErr = fmt.Errorf("gateway unavailable")
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}

	err := a.Start(context.Background(), handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gateway unavailable")
}

func TestStopWhenNotRunning(t *testing.T) {
	a := New("test-token")
	err := a.Stop(context.Background())
	assert.NoError(t, err)
}

// --- Send Tests ---

func TestSend(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	err := a.Send(context.Background(), channel.OutboundMessage{
		Content:     "hello discord",
		RecipientID: "123456",
	})
	require.NoError(t, err)

	calls := mock.getSendCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "123456", calls[0].channelID)
	assert.Equal(t, "hello discord", calls[0].content)

	_ = a.Stop(context.Background())
}

func TestSendWithReplyTo(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	err := a.Send(context.Background(), channel.OutboundMessage{
		Content:     "reply message",
		RecipientID: "chan-1",
		ReplyToID:   "msg-42",
	})
	require.NoError(t, err)

	// Should use ChannelMessageSendComplex for replies.
	complexCalls := mock.getComplexCalls()
	require.Len(t, complexCalls, 1)
	assert.Equal(t, "chan-1", complexCalls[0].channelID)
	assert.Equal(t, "reply message", complexCalls[0].data.Content)
	assert.Equal(t, "msg-42", complexCalls[0].data.Reference.MessageID)
	assert.Equal(t, "chan-1", complexCalls[0].data.Reference.ChannelID)

	// Regular send should not be called.
	sendCalls := mock.getSendCalls()
	assert.Empty(t, sendCalls)

	_ = a.Stop(context.Background())
}

func TestSendBeforeStart(t *testing.T) {
	a := New("test-token")
	err := a.Send(context.Background(), channel.OutboundMessage{
		Content:     "hello",
		RecipientID: "12345",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestSendError(t *testing.T) {
	mock := newMockSession()
	mock.sendErr = fmt.Errorf("rate limited")
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	err := a.Send(context.Background(), channel.OutboundMessage{
		Content:     "hello",
		RecipientID: "12345",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")

	_ = a.Stop(context.Background())
}

func TestSendComplexError(t *testing.T) {
	mock := newMockSession()
	mock.complexErr = fmt.Errorf("forbidden")
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	err := a.Send(context.Background(), channel.OutboundMessage{
		Content:     "reply",
		RecipientID: "chan-1",
		ReplyToID:   "msg-1",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "forbidden")

	_ = a.Stop(context.Background())
}

// --- HandleMessage Tests ---

func TestHandleMessageTextMessage(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	var mu sync.Mutex
	var received []channel.InboundMessage

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}

	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	// Simulate a message event via the registered handler.
	invokeMessageHandler(mock, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-100",
			ChannelID: "chan-1",
			GuildID:   "guild-1",
			Content:   "hello bot",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Author: &discordgo.User{
				ID:         "user-42",
				Username:   "johndoe",
				GlobalName: "John Doe",
			},
		},
	})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	msg := received[0]
	mu.Unlock()

	assert.Equal(t, "msg-100", msg.ID)
	assert.Equal(t, channel.TypeDiscord, msg.ChannelType)
	assert.Equal(t, "discord", msg.ChannelName)
	assert.Equal(t, "user-42", msg.SenderID)
	assert.Equal(t, "John Doe", msg.SenderName) // GlobalName preferred
	assert.Equal(t, "hello bot", msg.Content)
	assert.Empty(t, msg.ThreadID)
	assert.Empty(t, msg.ReplyToID)
	assert.Empty(t, msg.Attachments)
	assert.Equal(t, "chan-1", msg.Metadata["channel_id"])
	assert.Equal(t, "guild-1", msg.Metadata["guild_id"])
	assert.Equal(t, "johndoe", msg.Metadata["username"])

	_ = a.Stop(context.Background())
}

func TestHandleMessageUsernameWhenNoGlobalName(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	var mu sync.Mutex
	var received []channel.InboundMessage

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}

	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	invokeMessageHandler(mock, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-101",
			ChannelID: "chan-1",
			Content:   "hi",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Author: &discordgo.User{
				ID:       "user-7",
				Username: "alice",
				// GlobalName is empty — should fallback to Username.
			},
		},
	})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	assert.Equal(t, "alice", received[0].SenderName)
	mu.Unlock()

	_ = a.Stop(context.Background())
}

func TestHandleMessageWithAttachments(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	var mu sync.Mutex
	var received []channel.InboundMessage

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}

	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	invokeMessageHandler(mock, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-200",
			ChannelID: "chan-1",
			Content:   "check this out",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Author: &discordgo.User{
				ID:       "user-1",
				Username: "test",
			},
			Attachments: []*discordgo.MessageAttachment{
				{
					ID:          "att-1",
					URL:         "https://cdn.discord.com/photo.png",
					Filename:    "photo.png",
					ContentType: "image/png",
					Size:        4096,
				},
				{
					ID:          "att-2",
					URL:         "https://cdn.discord.com/doc.pdf",
					Filename:    "doc.pdf",
					ContentType: "application/pdf",
					Size:        10240,
				},
			},
		},
	})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	msg := received[0]
	mu.Unlock()

	assert.Equal(t, "check this out", msg.Content)
	require.Len(t, msg.Attachments, 2)

	assert.Equal(t, channel.AttachmentImage, msg.Attachments[0].Type)
	assert.Equal(t, "https://cdn.discord.com/photo.png", msg.Attachments[0].URL)
	assert.Equal(t, "photo.png", msg.Attachments[0].Filename)
	assert.Equal(t, "image/png", msg.Attachments[0].MIMEType)
	assert.Equal(t, int64(4096), msg.Attachments[0].Size)

	assert.Equal(t, channel.AttachmentDocument, msg.Attachments[1].Type)
	assert.Equal(t, "doc.pdf", msg.Attachments[1].Filename)

	_ = a.Stop(context.Background())
}

func TestHandleMessageSkipsBotAuthor(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	var mu sync.Mutex
	var received []channel.InboundMessage

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}

	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	// Bot message should be skipped.
	invokeMessageHandler(mock, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-300",
			ChannelID: "chan-1",
			Content:   "I am a bot",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Author: &discordgo.User{
				ID:       "bot-1",
				Username: "mybot",
				Bot:      true,
			},
		},
	})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Empty(t, received)
	mu.Unlock()

	_ = a.Stop(context.Background())
}

func TestHandleMessageSkipsNilAuthor(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	var mu sync.Mutex
	var received []channel.InboundMessage

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}

	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	invokeMessageHandler(mock, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-301",
			ChannelID: "chan-1",
			Content:   "no author",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Author:    nil,
		},
	})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Empty(t, received)
	mu.Unlock()

	_ = a.Stop(context.Background())
}

func TestHandleMessageSkipsEmptyContent(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	var mu sync.Mutex
	var received []channel.InboundMessage

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}

	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	// Message with no content and no attachments should be skipped.
	invokeMessageHandler(mock, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-400",
			ChannelID: "chan-1",
			Content:   "",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Author: &discordgo.User{
				ID:       "user-1",
				Username: "test",
			},
		},
	})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	assert.Empty(t, received)
	mu.Unlock()

	_ = a.Stop(context.Background())
}

func TestHandleMessageWithAttachmentsNoText(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	var mu sync.Mutex
	var received []channel.InboundMessage

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}

	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	// Message with no text but has attachments — should NOT be skipped.
	invokeMessageHandler(mock, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-401",
			ChannelID: "chan-1",
			Content:   "",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Author: &discordgo.User{
				ID:       "user-1",
				Username: "test",
			},
			Attachments: []*discordgo.MessageAttachment{
				{
					ID:          "att-1",
					URL:         "https://cdn.discord.com/image.png",
					Filename:    "image.png",
					ContentType: "image/png",
					Size:        1024,
				},
			},
		},
	})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	assert.Equal(t, "", received[0].Content)
	require.Len(t, received[0].Attachments, 1)
	mu.Unlock()

	_ = a.Stop(context.Background())
}

func TestHandleMessageWithReplyReference(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	var mu sync.Mutex
	var received []channel.InboundMessage

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}

	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	invokeMessageHandler(mock, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-500",
			ChannelID: "chan-1",
			Content:   "replying",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Author: &discordgo.User{
				ID:       "user-1",
				Username: "test",
			},
			MessageReference: &discordgo.MessageReference{
				MessageID: "msg-499",
				ChannelID: "chan-1",
				GuildID:   "guild-1",
			},
		},
	})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	assert.Equal(t, "msg-499", received[0].ReplyToID)
	mu.Unlock()

	_ = a.Stop(context.Background())
}

func TestHandleMessageInThread(t *testing.T) {
	mock := newMockSession()
	// Mark channel "thread-chan" as a thread.
	mock.channels["thread-chan"] = &discordgo.Channel{
		ID:   "thread-chan",
		Type: discordgo.ChannelTypeGuildPublicThread,
	}

	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	var mu sync.Mutex
	var received []channel.InboundMessage

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}

	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	invokeMessageHandler(mock, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-600",
			ChannelID: "thread-chan",
			GuildID:   "guild-1",
			Content:   "thread message",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Author: &discordgo.User{
				ID:       "user-1",
				Username: "test",
			},
		},
	})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	assert.Equal(t, "thread-chan", received[0].ThreadID)
	mu.Unlock()

	_ = a.Stop(context.Background())
}

func TestHandleMessageInNonThread(t *testing.T) {
	mock := newMockSession()
	// Regular text channel — not a thread.
	mock.channels["text-chan"] = &discordgo.Channel{
		ID:   "text-chan",
		Type: discordgo.ChannelTypeGuildText,
	}

	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	var mu sync.Mutex
	var received []channel.InboundMessage

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg)
		mu.Unlock()
		return nil
	}

	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	invokeMessageHandler(mock, &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ID:        "msg-601",
			ChannelID: "text-chan",
			Content:   "regular message",
			Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Author: &discordgo.User{
				ID:       "user-1",
				Username: "test",
			},
		},
	})

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	assert.Empty(t, received[0].ThreadID)
	mu.Unlock()

	_ = a.Stop(context.Background())
}

// --- Typing Tests ---

func TestSendTyping(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	err := a.SendTyping(context.Background(), "chan-1", channel.TypingActionTyping)
	require.NoError(t, err)

	calls := mock.getTypingCalls()
	require.Len(t, calls, 1)
	assert.Equal(t, "chan-1", calls[0])

	_ = a.Stop(context.Background())
}

func TestSendTypingAllActionsMapToSame(t *testing.T) {
	mock := newMockSession()
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	// All typing actions should map to the same Discord ChannelTyping call.
	actions := []channel.TypingAction{
		channel.TypingActionTyping,
		channel.TypingActionUpload,
		channel.TypingActionRecording,
	}

	for _, action := range actions {
		err := a.SendTyping(context.Background(), "chan-1", action)
		require.NoError(t, err)
	}

	calls := mock.getTypingCalls()
	assert.Len(t, calls, 3)

	_ = a.Stop(context.Background())
}

func TestSendTypingBeforeStart(t *testing.T) {
	a := New("test-token")
	err := a.SendTyping(context.Background(), "chan-1", channel.TypingActionTyping)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestSendTypingError(t *testing.T) {
	mock := newMockSession()
	mock.typingErr = fmt.Errorf("rate limited")
	a := New("test-token", WithSessionFactory(mockSessionFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	err := a.SendTyping(context.Background(), "chan-1", channel.TypingActionTyping)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "rate limited")

	_ = a.Stop(context.Background())
}

// --- Thread Tests ---

func TestBindThread(t *testing.T) {
	a := New("test-token")

	msg := channel.InboundMessage{
		ThreadID: "thread-123",
	}
	assert.Equal(t, "thread-123", a.BindThread(msg))
}

func TestBindThreadEmpty(t *testing.T) {
	a := New("test-token")

	msg := channel.InboundMessage{}
	assert.Empty(t, a.BindThread(msg))
}

// --- detectThreadID Tests ---

func TestDetectThreadIDPublicThread(t *testing.T) {
	mock := newMockSession()
	mock.channels["thread-1"] = &discordgo.Channel{
		ID:   "thread-1",
		Type: discordgo.ChannelTypeGuildPublicThread,
	}

	result := detectThreadID(mock, "thread-1")
	assert.Equal(t, "thread-1", result)
}

func TestDetectThreadIDPrivateThread(t *testing.T) {
	mock := newMockSession()
	mock.channels["thread-2"] = &discordgo.Channel{
		ID:   "thread-2",
		Type: discordgo.ChannelTypeGuildPrivateThread,
	}

	result := detectThreadID(mock, "thread-2")
	assert.Equal(t, "thread-2", result)
}

func TestDetectThreadIDNonThread(t *testing.T) {
	mock := newMockSession()
	mock.channels["chan-1"] = &discordgo.Channel{
		ID:   "chan-1",
		Type: discordgo.ChannelTypeGuildText,
	}

	result := detectThreadID(mock, "chan-1")
	assert.Empty(t, result)
}

func TestDetectThreadIDChannelError(t *testing.T) {
	mock := newMockSession()
	mock.channelErr = fmt.Errorf("unknown channel")

	result := detectThreadID(mock, "unknown")
	assert.Empty(t, result)
}

// --- buildMetadata Tests ---

func TestBuildMetadata(t *testing.T) {
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "chan-1",
			GuildID:   "guild-1",
			Author: &discordgo.User{
				Username: "testuser",
			},
		},
	}

	meta := buildMetadata(m)
	assert.Equal(t, "chan-1", meta["channel_id"])
	assert.Equal(t, "guild-1", meta["guild_id"])
	assert.Equal(t, "testuser", meta["username"])
}

func TestBuildMetadataWithoutGuild(t *testing.T) {
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "dm-1",
			GuildID:   "", // DM — no guild.
			Author: &discordgo.User{
				Username: "testuser",
			},
		},
	}

	meta := buildMetadata(m)
	assert.Equal(t, "dm-1", meta["channel_id"])
	_, hasGuild := meta["guild_id"]
	assert.False(t, hasGuild)
}

func TestBuildMetadataWithoutUsername(t *testing.T) {
	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "chan-1",
			Author:    &discordgo.User{},
		},
	}

	meta := buildMetadata(m)
	_, hasUsername := meta["username"]
	assert.False(t, hasUsername)
}

// --- classifyContentType Tests ---

func TestClassifyContentType(t *testing.T) {
	tests := []struct {
		contentType string
		want        channel.AttachmentType
	}{
		{"image/png", channel.AttachmentImage},
		{"image/jpeg", channel.AttachmentImage},
		{"IMAGE/GIF", channel.AttachmentImage}, // case insensitive
		{"audio/ogg", channel.AttachmentAudio},
		{"audio/mpeg", channel.AttachmentAudio},
		{"video/mp4", channel.AttachmentVideo},
		{"video/webm", channel.AttachmentVideo},
		{"application/pdf", channel.AttachmentDocument},
		{"text/plain", channel.AttachmentDocument},
		{"", channel.AttachmentDocument},
	}

	for _, tt := range tests {
		t.Run(tt.contentType, func(t *testing.T) {
			got := classifyContentType(tt.contentType)
			assert.Equal(t, tt.want, got)
		})
	}
}
