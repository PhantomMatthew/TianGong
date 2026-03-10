package channel

import (
	"context"
)

// TypingAction represents the kind of typing indicator to show.
type TypingAction string

const (
	// TypingActionTyping indicates the bot is composing a text reply.
	TypingActionTyping TypingAction = "typing"
	// TypingActionUpload indicates the bot is uploading media.
	TypingActionUpload TypingAction = "upload"
	// TypingActionRecording indicates the bot is recording audio/video.
	TypingActionRecording TypingAction = "recording"
)

// TypingIndicator is an optional interface that channel adapters can implement
// to show typing indicators (e.g., "bot is typing…") to the user.
//
// Not all channels support typing indicators. Callers should type-assert
// the adapter to check for this capability.
type TypingIndicator interface {
	// SendTyping sends a typing indicator to the given recipient.
	// The action parameter specifies the kind of indicator (typing, upload, etc.).
	// recipientID is the platform-specific user/chat identifier.
	SendTyping(ctx context.Context, recipientID string, action TypingAction) error
}
