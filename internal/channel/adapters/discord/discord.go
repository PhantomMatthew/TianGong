// Package discord provides a Discord channel adapter using discordgo.
//
// The adapter connects to Discord's Gateway via WebSocket to receive messages
// and uses the REST API to send replies. It implements Adapter, Receiver,
// Sender, TypingIndicator, and ThreadBinder from the channel package.
//
// Configuration requires a bot token from the Discord Developer Portal.
// The token is passed via ChannelConfig.Settings["token"].
//
// Wire protocol:
//   - Inbound: MessageCreate events via Gateway WebSocket
//   - Outbound: ChannelMessageSend / ChannelMessageSendReply via REST API
//   - Typing: ChannelTyping via REST API (lasts ~10s per call)
//   - Threads: Discord thread channels detected via Channel.IsThread()
package discord

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/bwmarrin/discordgo"

	"github.com/PhantomMatthew/TianGong/internal/channel"
)

const (
	// SettingToken is the config key for the Discord bot token.
	SettingToken = "token"
)

// Session defines the subset of discordgo.Session methods used by the adapter.
// This interface enables testing without a real Discord connection.
type Session interface {
	Open() error
	Close() error
	AddHandler(handler interface{}) func()
	ChannelMessageSend(channelID string, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelMessageSendComplex(channelID string, data *discordgo.MessageSend, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelTyping(channelID string, options ...discordgo.RequestOption) error
	Channel(channelID string, options ...discordgo.RequestOption) (*discordgo.Channel, error)
}

// SessionFactory creates a Session from a bot token. Replaceable for testing.
type SessionFactory func(token string) (Session, error)

// DefaultSessionFactory creates a real discordgo.Session.
func DefaultSessionFactory(token string) (Session, error) {
	s, err := discordgo.New("Bot " + token)
	if err != nil {
		return nil, fmt.Errorf("failed to create discord session: %w", err)
	}
	s.Identify.Intents = discordgo.IntentGuildMessages |
		discordgo.IntentDirectMessages |
		discordgo.IntentMessageContent
	return s, nil
}

// Adapter is a Discord channel adapter.
// It uses the Discord Gateway to receive messages and the REST API to send replies.
type Adapter struct {
	name           string
	token          string
	sessionFactory SessionFactory

	mu      sync.Mutex
	session Session
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// Option configures the Discord adapter.
type Option func(*Adapter)

// WithName sets the adapter instance name (default: "discord").
func WithName(name string) Option {
	return func(a *Adapter) {
		a.name = name
	}
}

// WithSessionFactory sets a custom session factory (primarily for testing).
func WithSessionFactory(f SessionFactory) Option {
	return func(a *Adapter) {
		a.sessionFactory = f
	}
}

// New creates a new Discord adapter.
// The token is the Discord bot token from the Developer Portal.
func New(token string, opts ...Option) *Adapter {
	a := &Adapter{
		name:           "discord",
		token:          token,
		sessionFactory: DefaultSessionFactory,
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// NewFromConfig creates a Discord adapter from a ChannelConfig.
// Expects Settings["token"] to contain the bot token.
func NewFromConfig(cfg channel.ChannelConfig, opts ...Option) (*Adapter, error) {
	token, ok := cfg.Settings[SettingToken]
	if !ok || token == "" {
		return nil, fmt.Errorf("discord adapter requires %q in settings", SettingToken)
	}

	allOpts := []Option{WithName(cfg.Name)}
	allOpts = append(allOpts, opts...)

	return New(token, allOpts...), nil
}

// Type returns the channel type identifier.
func (a *Adapter) Type() channel.ChannelType {
	return channel.TypeDiscord
}

// Name returns the adapter instance name.
func (a *Adapter) Name() string {
	return a.name
}

// Start begins receiving messages from Discord and dispatches them to the
// handler. It blocks until the context is cancelled or Stop is called.
func (a *Adapter) Start(ctx context.Context, handler channel.InboundHandler) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("discord adapter already running")
	}

	session, err := a.sessionFactory(a.token)
	if err != nil {
		a.mu.Unlock()
		return fmt.Errorf("failed to initialize session: %w", err)
	}

	a.session = session
	a.done = make(chan struct{})

	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.running = true
	a.mu.Unlock()

	// Register message handler before opening the connection.
	session.AddHandler(func(_ *discordgo.Session, m *discordgo.MessageCreate) {
		a.handleMessage(ctx, handler, session, m)
	})

	if err := session.Open(); err != nil {
		a.setRunning(false)
		cancel()
		return fmt.Errorf("failed to open discord connection: %w", err)
	}

	slog.Info("discord adapter started", "name", a.name)

	// Block until context is cancelled or Stop is called.
	stopped := false
	select {
	case <-ctx.Done():
	case <-a.done:
		stopped = true
	}

	if err := session.Close(); err != nil {
		slog.Warn("discord session close error", "error", err)
	}

	a.setRunning(false)

	if !stopped && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// Stop gracefully shuts down the adapter.
func (a *Adapter) Stop(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.running {
		return nil
	}

	close(a.done)
	if a.cancel != nil {
		a.cancel()
	}
	a.running = false
	slog.Info("discord adapter stopped", "name", a.name)
	return nil
}

// Send sends an outbound message to a Discord channel.
func (a *Adapter) Send(_ context.Context, msg channel.OutboundMessage) error {
	a.mu.Lock()
	s := a.session
	a.mu.Unlock()

	if s == nil {
		return fmt.Errorf("discord session not initialized (call Start first)")
	}

	// If a reply-to is specified, send as a reply.
	if msg.ReplyToID != "" {
		ref := &discordgo.MessageReference{
			MessageID: msg.ReplyToID,
			ChannelID: msg.RecipientID,
		}
		_, err := s.ChannelMessageSendComplex(msg.RecipientID, &discordgo.MessageSend{
			Content:   msg.Content,
			Reference: ref,
		})
		if err != nil {
			return fmt.Errorf("failed to send discord reply: %w", err)
		}
		return nil
	}

	_, err := s.ChannelMessageSend(msg.RecipientID, msg.Content)
	if err != nil {
		return fmt.Errorf("failed to send discord message: %w", err)
	}
	return nil
}

// SendTyping sends a typing indicator to a Discord channel.
func (a *Adapter) SendTyping(_ context.Context, recipientID string, _ channel.TypingAction) error {
	a.mu.Lock()
	s := a.session
	a.mu.Unlock()

	if s == nil {
		return fmt.Errorf("discord session not initialized (call Start first)")
	}

	// Discord only has one typing action — all actions map to ChannelTyping.
	if err := s.ChannelTyping(recipientID); err != nil {
		return fmt.Errorf("failed to send typing indicator: %w", err)
	}
	return nil
}

// BindThread returns the thread/channel ID for a reply.
// For Discord threads, messages already carry the thread channel ID.
func (a *Adapter) BindThread(msg channel.InboundMessage) string {
	return msg.ThreadID
}

// handleMessage converts a Discord MessageCreate event to an InboundMessage.
func (a *Adapter) handleMessage(ctx context.Context, handler channel.InboundHandler, s Session, m *discordgo.MessageCreate) {
	if m.Author == nil || m.Author.Bot {
		return
	}

	if m.Content == "" && len(m.Attachments) == 0 {
		return
	}

	// Build attachments.
	var attachments []channel.Attachment
	for _, att := range m.Attachments {
		attachments = append(attachments, channel.Attachment{
			Type:     classifyContentType(att.ContentType),
			URL:      att.URL,
			Filename: att.Filename,
			MIMEType: att.ContentType,
			Size:     int64(att.Size),
		})
	}

	// Determine sender name.
	senderName := m.Author.GlobalName
	if senderName == "" {
		senderName = m.Author.Username
	}

	// Determine thread ID: if the channel is a thread, use the channel ID.
	threadID := detectThreadID(s, m.ChannelID)

	// Determine reply-to.
	replyToID := ""
	if m.MessageReference != nil {
		replyToID = m.MessageReference.MessageID
	}

	inbound := channel.InboundMessage{
		ID:          m.ID,
		ChannelType: channel.TypeDiscord,
		ChannelName: a.name,
		SenderID:    m.Author.ID,
		SenderName:  senderName,
		Content:     m.Content,
		ThreadID:    threadID,
		ReplyToID:   replyToID,
		Attachments: attachments,
		Timestamp:   m.Timestamp,
		Metadata:    buildMetadata(m),
	}

	if err := handler(ctx, inbound); err != nil {
		slog.Error("discord handler error",
			"message_id", m.ID,
			"error", err,
		)
	}
}

// detectThreadID checks if a channel is a thread and returns the channel ID
// as the thread ID if so. Returns empty string for non-thread channels.
func detectThreadID(s Session, channelID string) string {
	ch, err := s.Channel(channelID)
	if err != nil {
		return ""
	}
	if ch.IsThread() {
		return channelID
	}
	return ""
}

// buildMetadata extracts useful metadata from a Discord message.
func buildMetadata(m *discordgo.MessageCreate) map[string]string {
	meta := map[string]string{
		"channel_id": m.ChannelID,
	}
	if m.GuildID != "" {
		meta["guild_id"] = m.GuildID
	}
	if m.Author.Username != "" {
		meta["username"] = m.Author.Username
	}
	return meta
}

// classifyContentType maps a MIME content type to an AttachmentType.
func classifyContentType(contentType string) channel.AttachmentType {
	ct := strings.ToLower(contentType)
	switch {
	case strings.HasPrefix(ct, "image/"):
		return channel.AttachmentImage
	case strings.HasPrefix(ct, "audio/"):
		return channel.AttachmentAudio
	case strings.HasPrefix(ct, "video/"):
		return channel.AttachmentVideo
	default:
		return channel.AttachmentDocument
	}
}

// setRunning safely sets the running flag.
func (a *Adapter) setRunning(v bool) {
	a.mu.Lock()
	a.running = v
	a.mu.Unlock()
}
