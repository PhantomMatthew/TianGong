// Package channel provides multi-channel messaging abstractions.
//
// The channel system enables TianGong to communicate across multiple
// messaging platforms (Telegram, Discord, Slack, CLI, etc.) through
// a unified adapter interface.
//
// Each channel implements one or more of these interfaces:
//   - Adapter: base identity (type + name)
//   - Receiver: can receive inbound messages
//   - Sender: can send outbound messages
//   - StreamingSender: can stream outbound messages (real-time LLM output)
package channel

import (
	"context"
)

// ChannelType identifies a messaging platform.
type ChannelType string

const (
	// TypeTelegram represents the Telegram channel.
	TypeTelegram ChannelType = "telegram"
	// TypeDiscord represents the Discord channel.
	TypeDiscord ChannelType = "discord"
	// TypeSlack represents the Slack channel.
	TypeSlack ChannelType = "slack"
	// TypeFeishu represents the Feishu/Lark channel.
	TypeFeishu ChannelType = "feishu"
	// TypeWhatsApp represents the WhatsApp channel.
	TypeWhatsApp ChannelType = "whatsapp"
	// TypeWeb represents the WebSocket web channel.
	TypeWeb ChannelType = "web"
	// TypeCLI represents the CLI channel (for dev/testing).
	TypeCLI ChannelType = "cli"
	// TypeMatrix represents the Matrix channel.
	TypeMatrix ChannelType = "matrix"
	// TypeMSTeams represents the Microsoft Teams channel.
	TypeMSTeams ChannelType = "msteams"
	// TypeSignal represents the Signal channel.
	TypeSignal ChannelType = "signal"
)

// Adapter is the base interface every channel must implement.
// It provides identity information for the channel.
type Adapter interface {
	// Type returns the channel type identifier.
	Type() ChannelType
	// Name returns a human-readable name for this channel instance.
	Name() string
}

// Receiver can receive inbound messages from a channel.
// Start begins listening for messages and dispatches them via the handler.
// Stop gracefully shuts down the receiver.
type Receiver interface {
	// Start begins listening for messages on this channel.
	// The handler is called for each inbound message.
	// This method should block until the context is cancelled or Stop is called.
	Start(ctx context.Context, handler InboundHandler) error
	// Stop gracefully shuts down the receiver.
	Stop(ctx context.Context) error
}

// Sender can send outbound messages to a channel.
type Sender interface {
	// Send sends a single outbound message to the channel.
	Send(ctx context.Context, msg OutboundMessage) error
}

// StreamingSender can stream outbound messages for real-time LLM output.
// This is used by channels that support incremental message updates
// (e.g., editing a message as the LLM generates tokens).
type StreamingSender interface {
	// SendStream opens a streaming outbound message.
	// The returned OutboundStream allows incremental writes and must be closed.
	SendStream(ctx context.Context, msg OutboundMessage) (OutboundStream, error)
}

// OutboundStream represents an open streaming message being sent.
// Callers write deltas and close when done.
type OutboundStream interface {
	// Write sends a text delta to the stream.
	Write(delta string) error
	// Close finalizes the streaming message.
	Close() error
}

// InboundHandler processes messages received from a channel.
// It is called by Receiver.Start for each inbound message.
type InboundHandler func(ctx context.Context, msg InboundMessage) error

// ChannelConfig holds per-channel instance configuration.
type ChannelConfig struct {
	// Type is the channel type.
	Type ChannelType `mapstructure:"type" json:"type" validate:"required"`
	// Name is a unique name for this channel instance.
	Name string `mapstructure:"name" json:"name" validate:"required"`
	// Enabled controls whether this channel is active.
	Enabled bool `mapstructure:"enabled" json:"enabled"`
	// Settings holds channel-specific configuration (API tokens, webhook URLs, etc.).
	Settings map[string]string `mapstructure:"settings" json:"settings,omitempty"`
}
