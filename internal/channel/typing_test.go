package channel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTypingAdapter implements Adapter, Sender, and TypingIndicator.
type mockTypingAdapter struct {
	mockAdapter
	typingCalls []typingCall
	typingErr   error
	sent        []OutboundMessage
}

type typingCall struct {
	recipientID string
	action      TypingAction
}

func (m *mockTypingAdapter) Send(_ context.Context, msg OutboundMessage) error {
	m.sent = append(m.sent, msg)
	return nil
}

func (m *mockTypingAdapter) SendTyping(_ context.Context, recipientID string, action TypingAction) error {
	m.typingCalls = append(m.typingCalls, typingCall{recipientID: recipientID, action: action})
	return m.typingErr
}

func TestTypingActionConstants(t *testing.T) {
	assert.Equal(t, TypingAction("typing"), TypingActionTyping)
	assert.Equal(t, TypingAction("upload"), TypingActionUpload)
	assert.Equal(t, TypingAction("recording"), TypingActionRecording)
}

func TestRegistryDetectsTypingIndicator(t *testing.T) {
	reg := NewRegistry()
	adapter := &mockTypingAdapter{
		mockAdapter: mockAdapter{channelType: TypeWeb, name: "web-1"},
	}
	cfg := ChannelConfig{Type: TypeWeb, Name: "web-1", Enabled: true}

	err := reg.Register(adapter, cfg)
	require.NoError(t, err)

	ti, ok := reg.GetTypingIndicator("web-1")
	assert.True(t, ok)
	assert.NotNil(t, ti)

	// Should be the same adapter.
	assert.Equal(t, adapter, ti)
}

func TestRegistryNoTypingIndicator(t *testing.T) {
	reg := NewRegistry()
	adapter := &mockAdapter{channelType: TypeCLI, name: "cli"}
	cfg := ChannelConfig{Type: TypeCLI, Name: "cli", Enabled: true}

	err := reg.Register(adapter, cfg)
	require.NoError(t, err)

	ti, ok := reg.GetTypingIndicator("cli")
	assert.False(t, ok)
	assert.Nil(t, ti)
}

func TestRegistryGetTypingIndicatorNotFound(t *testing.T) {
	reg := NewRegistry()
	ti, ok := reg.GetTypingIndicator("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, ti)
}

func TestTypingIndicatorSendsAction(t *testing.T) {
	adapter := &mockTypingAdapter{
		mockAdapter: mockAdapter{channelType: TypeWeb, name: "web-1"},
	}

	err := adapter.SendTyping(context.Background(), "user-42", TypingActionTyping)
	require.NoError(t, err)

	require.Len(t, adapter.typingCalls, 1)
	assert.Equal(t, "user-42", adapter.typingCalls[0].recipientID)
	assert.Equal(t, TypingActionTyping, adapter.typingCalls[0].action)
}

func TestTypingIndicatorUploadAction(t *testing.T) {
	adapter := &mockTypingAdapter{
		mockAdapter: mockAdapter{channelType: TypeTelegram, name: "tg"},
	}

	err := adapter.SendTyping(context.Background(), "chat-99", TypingActionUpload)
	require.NoError(t, err)

	require.Len(t, adapter.typingCalls, 1)
	assert.Equal(t, TypingActionUpload, adapter.typingCalls[0].action)
}

func TestTypingIndicatorRecordingAction(t *testing.T) {
	adapter := &mockTypingAdapter{
		mockAdapter: mockAdapter{channelType: TypeTelegram, name: "tg"},
	}

	err := adapter.SendTyping(context.Background(), "chat-99", TypingActionRecording)
	require.NoError(t, err)

	require.Len(t, adapter.typingCalls, 1)
	assert.Equal(t, TypingActionRecording, adapter.typingCalls[0].action)
}
