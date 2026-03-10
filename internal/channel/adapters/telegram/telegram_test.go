package telegram

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PhantomMatthew/TianGong/internal/channel"
)

// mockBot implements BotAPI for testing.
type mockBot struct {
	mu       sync.Mutex
	updates  chan tgbotapi.Update
	sent     []tgbotapi.Chattable
	stopped  bool
	sendFunc func(tgbotapi.Chattable) (tgbotapi.Message, error)
}

func newMockBot() *mockBot {
	return &mockBot{
		updates: make(chan tgbotapi.Update, 64),
	}
}

func (m *mockBot) GetUpdatesChan(_ tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel {
	return m.updates
}

func (m *mockBot) StopReceivingUpdates() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.stopped {
		m.stopped = true
		close(m.updates)
	}
}

func (m *mockBot) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, c)
	if m.sendFunc != nil {
		return m.sendFunc(c)
	}
	return tgbotapi.Message{MessageID: len(m.sent)}, nil
}

func (m *mockBot) Request(_ tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	return &tgbotapi.APIResponse{Ok: true}, nil
}

func (m *mockBot) getSent() []tgbotapi.Chattable {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]tgbotapi.Chattable, len(m.sent))
	copy(cp, m.sent)
	return cp
}

// mockBotFactory returns a BotFactory that injects the given mock.
func mockBotFactory(bot BotAPI) BotFactory {
	return func(_ string) (BotAPI, error) {
		return bot, nil
	}
}

// failingBotFactory returns a BotFactory that always fails.
func failingBotFactory() BotFactory {
	return func(_ string) (BotAPI, error) {
		return nil, fmt.Errorf("connection refused")
	}
}

func TestAdapterTypeAndName(t *testing.T) {
	a := New("test-token")
	assert.Equal(t, channel.TypeTelegram, a.Type())
	assert.Equal(t, "telegram", a.Name())
}

func TestAdapterCustomName(t *testing.T) {
	a := New("test-token", WithName("my-bot"))
	assert.Equal(t, "my-bot", a.Name())
}

func TestAdapterImplementsInterfaces(t *testing.T) {
	a := New("test-token")
	var _ channel.Adapter = a
	var _ channel.Receiver = a
	var _ channel.Sender = a
}

func TestNewFromConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     channel.ChannelConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: channel.ChannelConfig{
				Name:     "prod-bot",
				Type:     channel.TypeTelegram,
				Settings: map[string]string{"token": "abc:123"},
			},
			wantErr: false,
		},
		{
			name: "missing token",
			cfg: channel.ChannelConfig{
				Name:     "no-token",
				Type:     channel.TypeTelegram,
				Settings: map[string]string{},
			},
			wantErr: true,
		},
		{
			name: "empty token",
			cfg: channel.ChannelConfig{
				Name:     "empty-token",
				Type:     channel.TypeTelegram,
				Settings: map[string]string{"token": ""},
			},
			wantErr: true,
		},
		{
			name: "nil settings",
			cfg: channel.ChannelConfig{
				Name: "nil-settings",
				Type: channel.TypeTelegram,
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
				assert.Equal(t, channel.TypeTelegram, a.Type())
			}
		})
	}
}

func TestStartAndStop(t *testing.T) {
	mock := newMockBot()
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Start(context.Background(), handler)
	}()

	// Give Start time to begin polling.
	time.Sleep(50 * time.Millisecond)

	err := a.Stop(context.Background())
	require.NoError(t, err)

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

func TestStartStopsOnContextCancel(t *testing.T) {
	mock := newMockBot()
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

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
	mock := newMockBot()
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

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

func TestStartBotFactoryError(t *testing.T) {
	a := New("test-token", WithBotFactory(failingBotFactory()))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}

	err := a.Start(context.Background(), handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "connection refused")
}

func TestStopWhenNotRunning(t *testing.T) {
	a := New("test-token")
	err := a.Stop(context.Background())
	assert.NoError(t, err)
}

func TestSend(t *testing.T) {
	mock := newMockBot()
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

	// Start the adapter so bot is initialized.
	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	err := a.Send(context.Background(), channel.OutboundMessage{
		Content:     "hello world",
		RecipientID: "12345",
	})
	require.NoError(t, err)

	sent := mock.getSent()
	require.Len(t, sent, 1)

	msg, ok := sent[0].(tgbotapi.MessageConfig)
	require.True(t, ok)
	assert.Equal(t, int64(12345), msg.ChatID)
	assert.Equal(t, "hello world", msg.Text)
	assert.Equal(t, tgbotapi.ModeMarkdown, msg.ParseMode)

	_ = a.Stop(context.Background())
}

func TestSendWithReplyTo(t *testing.T) {
	mock := newMockBot()
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	err := a.Send(context.Background(), channel.OutboundMessage{
		Content:     "reply",
		RecipientID: "12345",
		ReplyToID:   "42",
	})
	require.NoError(t, err)

	sent := mock.getSent()
	require.Len(t, sent, 1)

	msg, ok := sent[0].(tgbotapi.MessageConfig)
	require.True(t, ok)
	assert.Equal(t, 42, msg.ReplyToMessageID)

	_ = a.Stop(context.Background())
}

func TestSendInvalidRecipientID(t *testing.T) {
	mock := newMockBot()
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}
	go func() {
		_ = a.Start(context.Background(), handler)
	}()
	time.Sleep(50 * time.Millisecond)

	err := a.Send(context.Background(), channel.OutboundMessage{
		Content:     "hello",
		RecipientID: "not-a-number",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid recipient ID")

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

func TestSendBotError(t *testing.T) {
	mock := newMockBot()
	mock.sendFunc = func(_ tgbotapi.Chattable) (tgbotapi.Message, error) {
		return tgbotapi.Message{}, fmt.Errorf("rate limited")
	}
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

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

func TestHandleUpdateTextMessage(t *testing.T) {
	mock := newMockBot()
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

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

	// Send a text update.
	mock.updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 100,
			Text:      "hello bot",
			Date:      1700000000,
			Chat: &tgbotapi.Chat{
				ID:   999,
				Type: "private",
			},
			From: &tgbotapi.User{
				ID:           42,
				FirstName:    "John",
				LastName:     "Doe",
				UserName:     "johndoe",
				LanguageCode: "en",
			},
		},
	}

	// Wait for handler to process.
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	msg := received[0]
	mu.Unlock()

	assert.Equal(t, "100", msg.ID)
	assert.Equal(t, channel.TypeTelegram, msg.ChannelType)
	assert.Equal(t, "telegram", msg.ChannelName)
	assert.Equal(t, "42", msg.SenderID)
	assert.Equal(t, "John Doe", msg.SenderName)
	assert.Equal(t, "hello bot", msg.Content)
	assert.Empty(t, msg.ThreadID) // private chat, no thread
	assert.Empty(t, msg.Attachments)
	assert.Equal(t, "999", msg.Metadata["chat_id"])
	assert.Equal(t, "private", msg.Metadata["chat_type"])
	assert.Equal(t, "johndoe", msg.Metadata["username"])
	assert.Equal(t, "en", msg.Metadata["language"])
	assert.False(t, msg.Timestamp.IsZero())

	_ = a.Stop(context.Background())
}

func TestHandleUpdateGroupMessage(t *testing.T) {
	mock := newMockBot()
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

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

	mock.updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 200,
			Text:      "group msg",
			Date:      1700000000,
			Chat: &tgbotapi.Chat{
				ID:   -100123,
				Type: "supergroup",
			},
			From: &tgbotapi.User{
				ID:        7,
				FirstName: "Alice",
			},
		},
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	msg := received[0]
	mu.Unlock()

	assert.Equal(t, "-100123", msg.ThreadID) // group chat sets threadID
	assert.Equal(t, "Alice", msg.SenderName)

	_ = a.Stop(context.Background())
}

func TestHandleUpdateWithPhoto(t *testing.T) {
	mock := newMockBot()
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

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

	mock.updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 300,
			Caption:   "look at this",
			Date:      1700000000,
			Photo: []tgbotapi.PhotoSize{
				{FileID: "small-id", Width: 100, Height: 100},
				{FileID: "large-id", Width: 800, Height: 600},
			},
			Chat: &tgbotapi.Chat{ID: 1, Type: "private"},
			From: &tgbotapi.User{ID: 1, FirstName: "Test"},
		},
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	msg := received[0]
	mu.Unlock()

	assert.Equal(t, "look at this", msg.Content) // caption used when text is empty
	require.Len(t, msg.Attachments, 1)
	assert.Equal(t, channel.AttachmentImage, msg.Attachments[0].Type)
	assert.Equal(t, "large-id", msg.Attachments[0].URL) // largest photo selected

	_ = a.Stop(context.Background())
}

func TestHandleUpdateWithDocument(t *testing.T) {
	mock := newMockBot()
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

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

	mock.updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 400,
			Caption:   "doc here",
			Date:      1700000000,
			Document: &tgbotapi.Document{
				FileID:   "doc-file-id",
				MimeType: "application/pdf",
				FileName: "report.pdf",
				FileSize: 1024,
			},
			Chat: &tgbotapi.Chat{ID: 1, Type: "private"},
			From: &tgbotapi.User{ID: 1, FirstName: "Test"},
		},
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	msg := received[0]
	mu.Unlock()

	require.Len(t, msg.Attachments, 1)
	att := msg.Attachments[0]
	assert.Equal(t, channel.AttachmentDocument, att.Type)
	assert.Equal(t, "doc-file-id", att.URL)
	assert.Equal(t, "application/pdf", att.MIMEType)
	assert.Equal(t, "report.pdf", att.Filename)
	assert.Equal(t, int64(1024), att.Size)

	_ = a.Stop(context.Background())
}

func TestHandleUpdateWithVoice(t *testing.T) {
	mock := newMockBot()
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

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

	// Voice messages need a caption to pass the content check.
	mock.updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 500,
			Caption:   "voice note",
			Date:      1700000000,
			Voice: &tgbotapi.Voice{
				FileID:   "voice-file-id",
				MimeType: "audio/ogg",
				FileSize: 2048,
			},
			Chat: &tgbotapi.Chat{ID: 1, Type: "private"},
			From: &tgbotapi.User{ID: 1, FirstName: "Test"},
		},
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	att := received[0].Attachments
	mu.Unlock()

	require.Len(t, att, 1)
	assert.Equal(t, channel.AttachmentVoice, att[0].Type)
	assert.Equal(t, "voice-file-id", att[0].URL)
	assert.Equal(t, "audio/ogg", att[0].MIMEType)

	_ = a.Stop(context.Background())
}

func TestHandleUpdateSkipsNilMessage(t *testing.T) {
	mock := newMockBot()
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

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

	// Update with nil message should be skipped.
	mock.updates <- tgbotapi.Update{Message: nil}

	// Also send a real message to prove the adapter is working.
	mock.updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 600,
			Text:      "real message",
			Date:      1700000000,
			Chat:      &tgbotapi.Chat{ID: 1, Type: "private"},
			From:      &tgbotapi.User{ID: 1, FirstName: "Test"},
		},
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	assert.Equal(t, "real message", received[0].Content)
	mu.Unlock()

	_ = a.Stop(context.Background())
}

func TestHandleUpdateSkipsEmptyContent(t *testing.T) {
	mock := newMockBot()
	a := New("test-token", WithBotFactory(mockBotFactory(mock)))

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

	// Message with no text and no caption — should be skipped.
	mock.updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 700,
			Date:      1700000000,
			Chat:      &tgbotapi.Chat{ID: 1, Type: "private"},
			From:      &tgbotapi.User{ID: 1, FirstName: "Test"},
		},
	}

	// Real message to prove the handler works.
	mock.updates <- tgbotapi.Update{
		Message: &tgbotapi.Message{
			MessageID: 701,
			Text:      "has content",
			Date:      1700000000,
			Chat:      &tgbotapi.Chat{ID: 1, Type: "private"},
			From:      &tgbotapi.User{ID: 1, FirstName: "Test"},
		},
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Len(t, received, 1)
	assert.Equal(t, "has content", received[0].Content)
	mu.Unlock()

	_ = a.Stop(context.Background())
}

func TestParseChatID(t *testing.T) {
	tests := []struct {
		input   string
		want    int64
		wantErr bool
	}{
		{"12345", 12345, false},
		{"-100123", -100123, false},
		{"0", 0, false},
		{"not-a-number", 0, true},
		{"", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := parseChatID(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestBuildMetadata(t *testing.T) {
	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{
			ID:   999,
			Type: "private",
		},
		From: &tgbotapi.User{
			UserName:     "testuser",
			LanguageCode: "ja",
		},
	}

	meta := buildMetadata(msg)
	assert.Equal(t, "999", meta["chat_id"])
	assert.Equal(t, "private", meta["chat_type"])
	assert.Equal(t, "testuser", meta["username"])
	assert.Equal(t, "ja", meta["language"])
}

func TestBuildMetadataWithoutOptionalFields(t *testing.T) {
	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{
			ID:   1,
			Type: "group",
		},
		From: &tgbotapi.User{},
	}

	meta := buildMetadata(msg)
	assert.Equal(t, "1", meta["chat_id"])
	assert.Equal(t, "group", meta["chat_type"])
	_, hasUsername := meta["username"]
	_, hasLanguage := meta["language"]
	assert.False(t, hasUsername)
	assert.False(t, hasLanguage)
}
