// Package telegram provides a Telegram channel adapter using the Bot API.
//
// The adapter uses long-polling to receive messages and the Bot API to send
// replies. It implements Adapter, Receiver, Sender, TypingIndicator, and
// ThreadBinder from the channel package.
//
// Configuration requires a bot token obtained from @BotFather. The token is
// passed via ChannelConfig.Settings["token"].
package telegram

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/PhantomMatthew/TianGong/internal/channel"
)

const (
	// SettingToken is the config key for the Telegram bot token.
	SettingToken = "token"
	// DefaultPollTimeout is the default long-poll timeout in seconds.
	DefaultPollTimeout = 30
)

// BotAPI defines the subset of tgbotapi.BotAPI methods used by the adapter.
// This interface enables testing without a real Telegram connection.
type BotAPI interface {
	GetUpdatesChan(config tgbotapi.UpdateConfig) tgbotapi.UpdatesChannel
	StopReceivingUpdates()
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

// BotFactory creates a BotAPI from a token. Replaceable for testing.
type BotFactory func(token string) (BotAPI, error)

// DefaultBotFactory creates a real tgbotapi.BotAPI.
func DefaultBotFactory(token string) (BotAPI, error) {
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		return nil, fmt.Errorf("failed to create telegram bot: %w", err)
	}
	return bot, nil
}

// Adapter is a Telegram channel adapter.
// It uses long-polling to receive updates and the Bot API to send messages.
type Adapter struct {
	name       string
	token      string
	botFactory BotFactory

	mu      sync.Mutex
	bot     BotAPI
	running bool
	stopCh  chan struct{}
}

// Option configures the Telegram adapter.
type Option func(*Adapter)

// WithName sets the adapter instance name (default: "telegram").
func WithName(name string) Option {
	return func(a *Adapter) {
		a.name = name
	}
}

// WithBotFactory sets a custom bot factory (primarily for testing).
func WithBotFactory(f BotFactory) Option {
	return func(a *Adapter) {
		a.botFactory = f
	}
}

// New creates a new Telegram adapter.
// The token is the Telegram Bot API token from @BotFather.
func New(token string, opts ...Option) *Adapter {
	a := &Adapter{
		name:       "telegram",
		token:      token,
		botFactory: DefaultBotFactory,
		stopCh:     make(chan struct{}),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// NewFromConfig creates a Telegram adapter from a ChannelConfig.
// Expects Settings["token"] to contain the bot token.
func NewFromConfig(cfg channel.ChannelConfig, opts ...Option) (*Adapter, error) {
	token, ok := cfg.Settings[SettingToken]
	if !ok || token == "" {
		return nil, fmt.Errorf("telegram adapter requires %q in settings", SettingToken)
	}

	allOpts := []Option{WithName(cfg.Name)}
	allOpts = append(allOpts, opts...)

	return New(token, allOpts...), nil
}

// Type returns the channel type identifier.
func (a *Adapter) Type() channel.ChannelType {
	return channel.TypeTelegram
}

// Name returns the adapter instance name.
func (a *Adapter) Name() string {
	return a.name
}

// Start begins receiving messages via long-polling and dispatches them
// to the handler. It blocks until the context is cancelled or Stop is called.
func (a *Adapter) Start(ctx context.Context, handler channel.InboundHandler) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("telegram adapter already running")
	}

	bot, err := a.botFactory(a.token)
	if err != nil {
		a.mu.Unlock()
		return fmt.Errorf("failed to initialize bot: %w", err)
	}
	a.bot = bot
	a.running = true
	a.mu.Unlock()

	slog.Info("telegram adapter started", "name", a.name)

	updateConfig := tgbotapi.NewUpdate(0)
	updateConfig.Timeout = DefaultPollTimeout

	updates := bot.GetUpdatesChan(updateConfig)

	for {
		select {
		case <-ctx.Done():
			bot.StopReceivingUpdates()
			a.setRunning(false)
			return ctx.Err()
		case <-a.stopCh:
			bot.StopReceivingUpdates()
			a.setRunning(false)
			return nil
		case update, ok := <-updates:
			if !ok {
				a.setRunning(false)
				return nil
			}
			if update.Message == nil {
				continue
			}
			a.handleUpdate(ctx, handler, update)
		}
	}
}

// Stop gracefully shuts down the adapter.
func (a *Adapter) Stop(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.running {
		return nil
	}

	close(a.stopCh)
	a.running = false
	slog.Info("telegram adapter stopped", "name", a.name)
	return nil
}

// Send sends an outbound message to a Telegram chat.
func (a *Adapter) Send(_ context.Context, msg channel.OutboundMessage) error {
	a.mu.Lock()
	bot := a.bot
	a.mu.Unlock()

	if bot == nil {
		return fmt.Errorf("telegram bot not initialized (call Start first)")
	}

	chatID, err := parseChatID(msg.RecipientID)
	if err != nil {
		return fmt.Errorf("invalid recipient ID %q: %w", msg.RecipientID, err)
	}

	tgMsg := tgbotapi.NewMessage(chatID, msg.Content)
	tgMsg.ParseMode = tgbotapi.ModeMarkdown

	// Set reply-to if specified.
	if msg.ReplyToID != "" {
		replyID, parseErr := strconv.Atoi(msg.ReplyToID)
		if parseErr == nil {
			tgMsg.ReplyToMessageID = replyID
		}
	}

	_, err = bot.Send(tgMsg)
	if err != nil {
		return fmt.Errorf("failed to send telegram message: %w", err)
	}

	return nil
}

// handleUpdate converts a Telegram update to an InboundMessage and dispatches it.
func (a *Adapter) handleUpdate(ctx context.Context, handler channel.InboundHandler, update tgbotapi.Update) {
	msg := update.Message
	if msg == nil {
		return
	}

	// Build content from text or caption.
	content := msg.Text
	if content == "" {
		content = msg.Caption
	}
	if content == "" {
		return // Skip non-text messages for now.
	}

	// Build attachments from photos.
	var attachments []channel.Attachment
	if len(msg.Photo) > 0 {
		// Telegram sends multiple sizes; use the largest.
		largest := msg.Photo[len(msg.Photo)-1]
		attachments = append(attachments, channel.Attachment{
			Type: channel.AttachmentImage,
			URL:  largest.FileID,
		})
	}
	if msg.Document != nil {
		attachments = append(attachments, channel.Attachment{
			Type:     channel.AttachmentDocument,
			URL:      msg.Document.FileID,
			MIMEType: msg.Document.MimeType,
			Filename: msg.Document.FileName,
			Size:     int64(msg.Document.FileSize),
		})
	}
	if msg.Voice != nil {
		attachments = append(attachments, channel.Attachment{
			Type:     channel.AttachmentVoice,
			URL:      msg.Voice.FileID,
			MIMEType: msg.Voice.MimeType,
			Size:     int64(msg.Voice.FileSize),
		})
	}
	if msg.Audio != nil {
		attachments = append(attachments, channel.Attachment{
			Type:     channel.AttachmentAudio,
			URL:      msg.Audio.FileID,
			MIMEType: msg.Audio.MimeType,
			Filename: msg.Audio.FileName,
			Size:     int64(msg.Audio.FileSize),
		})
	}
	if msg.Video != nil {
		attachments = append(attachments, channel.Attachment{
			Type:     channel.AttachmentVideo,
			URL:      msg.Video.FileID,
			MIMEType: msg.Video.MimeType,
			Size:     int64(msg.Video.FileSize),
		})
	}

	// Determine sender name.
	senderName := strings.TrimSpace(msg.From.FirstName + " " + msg.From.LastName)
	if senderName == "" {
		senderName = msg.From.UserName
	}

	// Build thread ID from chat for group chats.
	threadID := ""
	if msg.Chat.IsGroup() || msg.Chat.IsSuperGroup() {
		threadID = strconv.FormatInt(msg.Chat.ID, 10)
	}

	inbound := channel.InboundMessage{
		ID:          strconv.Itoa(msg.MessageID),
		ChannelType: channel.TypeTelegram,
		ChannelName: a.name,
		SenderID:    strconv.FormatInt(msg.From.ID, 10),
		SenderName:  senderName,
		Content:     content,
		ThreadID:    threadID,
		Attachments: attachments,
		Timestamp:   time.Unix(int64(msg.Date), 0),
		Metadata:    buildMetadata(msg),
	}

	if err := handler(ctx, inbound); err != nil {
		slog.Error("telegram handler error",
			"message_id", msg.MessageID,
			"error", err,
		)
	}
}

// buildMetadata extracts useful metadata from a Telegram message.
func buildMetadata(msg *tgbotapi.Message) map[string]string {
	meta := map[string]string{
		"chat_id":   strconv.FormatInt(msg.Chat.ID, 10),
		"chat_type": msg.Chat.Type,
	}
	if msg.From.UserName != "" {
		meta["username"] = msg.From.UserName
	}
	if msg.From.LanguageCode != "" {
		meta["language"] = msg.From.LanguageCode
	}
	return meta
}

// parseChatID parses a recipient ID string to int64.
func parseChatID(id string) (int64, error) {
	return strconv.ParseInt(id, 10, 64)
}

// setRunning safely sets the running flag.
func (a *Adapter) setRunning(v bool) {
	a.mu.Lock()
	a.running = v
	a.mu.Unlock()
}

// SendTyping sends a typing indicator (chat action) to a Telegram chat.
// It maps TypingAction to Telegram-specific chat actions.
func (a *Adapter) SendTyping(_ context.Context, recipientID string, action channel.TypingAction) error {
	a.mu.Lock()
	bot := a.bot
	a.mu.Unlock()

	if bot == nil {
		return fmt.Errorf("telegram bot not initialized (call Start first)")
	}

	chatID, err := parseChatID(recipientID)
	if err != nil {
		return fmt.Errorf("invalid recipient ID %q: %w", recipientID, err)
	}

	tgAction := mapTypingAction(action)
	chatAction := tgbotapi.NewChatAction(chatID, tgAction)
	_, err = bot.Request(chatAction)
	if err != nil {
		return fmt.Errorf("failed to send typing action: %w", err)
	}

	return nil
}

// BindThread returns the thread ID for a reply.
// For group chats, Telegram uses the chat ID as the thread context.
// For private chats, there is no thread — returns empty string.
func (a *Adapter) BindThread(msg channel.InboundMessage) string {
	return msg.ThreadID
}

// mapTypingAction converts a channel.TypingAction to a Telegram chat action string.
func mapTypingAction(action channel.TypingAction) string {
	switch action {
	case channel.TypingActionUpload:
		return tgbotapi.ChatUploadDocument
	case channel.TypingActionRecording:
		return tgbotapi.ChatRecordVoice
	default:
		return tgbotapi.ChatTyping
	}
}
