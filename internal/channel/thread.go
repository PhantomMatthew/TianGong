package channel

import (
	"fmt"
)

// ThreadContext carries thread-related metadata for a message.
// It is used by routers and session resolvers to maintain thread-aware
// conversations (e.g., replying in a Telegram group thread, Discord channel thread).
type ThreadContext struct {
	// ThreadID is the platform-specific thread identifier.
	// For Telegram groups this is the chat ID; for Discord it is the thread snowflake.
	ThreadID string
	// ParentMessageID is the ID of the message that started the thread (if known).
	ParentMessageID string
	// IsThread indicates whether this message belongs to a thread.
	IsThread bool
}

// ThreadContextFromMessage extracts thread context from an InboundMessage.
func ThreadContextFromMessage(msg InboundMessage) ThreadContext {
	return ThreadContext{
		ThreadID:        msg.ThreadID,
		ParentMessageID: msg.ReplyToID,
		IsThread:        msg.ThreadID != "",
	}
}

// ThreadBinder is an optional interface that channel adapters can implement
// to provide thread-aware message binding.
//
// Adapters that support threads (Telegram groups, Discord threads, Slack threads)
// implement this to enable the router to reply within the correct thread context.
type ThreadBinder interface {
	// BindThread returns the thread ID for a reply given the inbound message context.
	// This allows the router to automatically set ThreadID on outbound messages.
	BindThread(msg InboundMessage) string
}

// ThreadAwareSessionResolver creates a SessionResolver that scopes sessions
// per thread when a thread is present, falling back to per-user sessions otherwise.
//
// Thread-scoped: "{channelType}:{threadID}:{senderID}"
// User-scoped:   "{channelType}:{senderID}"
func ThreadAwareSessionResolver(msg InboundMessage) string {
	if msg.ThreadID != "" {
		return fmt.Sprintf("%s:%s:%s", msg.ChannelType, msg.ThreadID, msg.SenderID)
	}
	return fmt.Sprintf("%s:%s", msg.ChannelType, msg.SenderID)
}
