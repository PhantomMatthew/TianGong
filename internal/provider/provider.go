// Package provider provides LLM provider abstractions.
package provider

import (
	"context"
	"errors"
	"fmt"
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

// FinishReason indicates why the model stopped generating.
type FinishReason string

const (
	// FinishReasonStop indicates the model finished normally.
	FinishReasonStop FinishReason = "stop"
	// FinishReasonToolCalls indicates the model wants to call tools.
	FinishReasonToolCalls FinishReason = "tool_calls"
	// FinishReasonMaxTokens indicates the response was truncated.
	FinishReasonMaxTokens FinishReason = "max_tokens"
)

// Provider defines the interface for LLM providers.
type Provider interface {
	// Name returns the provider identifier (e.g., "openai", "anthropic").
	Name() string
	// Chat sends a chat completion request and returns the full response.
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)
	// ChatStream sends a chat completion request and returns a stream of chunks.
	ChatStream(ctx context.Context, req *ChatRequest) (<-chan ChatChunk, error)
}

// ChatRequest contains input for a chat completion request.
type ChatRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	MaxTokens   int              `json:"max_tokens,omitempty"`
	Temperature *float64         `json:"temperature,omitempty"`
}

// ChatResponse contains the result of a chat completion request.
type ChatResponse struct {
	ID           string
	Content      string
	ToolCalls    []ToolCall
	Usage        Usage
	FinishReason FinishReason
}

// ChatChunk represents a single chunk in a streaming response.
type ChatChunk struct {
	Delta        string
	ToolCalls    []ToolCall
	Done         bool
	Error        error
	Usage        *Usage
	FinishReason FinishReason
}

// Message represents a message in a conversation.
type Message struct {
	Role       MessageRole `json:"role"`
	Content    string      `json:"content"`
	ToolCallID string      `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall  `json:"tool_calls,omitempty"`
}

// ToolCall represents a tool invocation requested by the model.
type ToolCall struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDefinition describes a tool available to the model.
type ToolDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// Usage contains token usage statistics.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Sentinel errors for provider failures.
var (
	// ErrAuthentication indicates an authentication failure (invalid API key).
	ErrAuthentication = errors.New("authentication failed")
	// ErrRateLimit indicates the provider rate limit was exceeded.
	ErrRateLimit = errors.New("rate limit exceeded")
	// ErrContextLength indicates the request exceeded the model's context window.
	ErrContextLength = errors.New("context length exceeded")
	// ErrInvalidRequest indicates the request was malformed.
	ErrInvalidRequest = errors.New("invalid request")
)

// ProviderError represents an error from an LLM provider.
type ProviderError struct {
	Code     string
	Message  string
	Provider string
	Err      error
}

// Error returns the formatted error string.
func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider %s: %s", e.Provider, e.Message)
}

// Unwrap returns the underlying error.
func (e *ProviderError) Unwrap() error {
	return e.Err
}
