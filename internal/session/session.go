// Package session provides conversation session management.
package session

import (
	"context"
	"time"
)

// MessageRole represents the role of a message in a conversation.
type MessageRole string

const (
	// RoleUser represents a user message.
	RoleUser MessageRole = "user"
	// RoleAssistant represents an assistant message.
	RoleAssistant MessageRole = "assistant"
	// RoleSystem represents a system message.
	RoleSystem MessageRole = "system"
	// RoleTool represents a tool result message.
	RoleTool MessageRole = "tool"
)

// Session represents a conversation session.
type Session struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

// Message represents a message within a session.
type Message struct {
	ID         string      `json:"id"`
	SessionID  string      `json:"session_id"`
	Role       MessageRole `json:"role"`
	Content    string      `json:"content"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
	CreatedAt  time.Time   `json:"created_at"`
}

// ToolCall represents a tool invocation requested by the model.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// SessionStore defines the interface for session persistence.
type SessionStore interface {
	// CreateSession creates a new session with the given title.
	CreateSession(ctx context.Context, title string) (*Session, error)
	// GetSession retrieves a session by ID.
	GetSession(ctx context.Context, id string) (*Session, error)
	// ListSessions returns all sessions ordered by most recently updated.
	ListSessions(ctx context.Context) ([]*Session, error)
	// AddMessage adds a message to a session.
	AddMessage(ctx context.Context, sessionID string, msg *Message) error
	// GetMessages returns all messages for a session in chronological order.
	GetMessages(ctx context.Context, sessionID string) ([]*Message, error)
}
