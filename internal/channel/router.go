package channel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/PhantomMatthew/TianGong/internal/bus"
)

// RouteHandler processes an inbound message and returns the response content.
// This is typically the agent's RunStream or equivalent.
type RouteHandler func(ctx context.Context, sessionID string, content string) (string, error)

// SessionResolver maps an inbound message to a session ID.
// This allows channels to maintain per-user or per-thread sessions.
type SessionResolver func(msg InboundMessage) string

// Router routes messages between channels and the agent.
// It processes inbound messages, resolves sessions, invokes the agent,
// and sends responses back through the appropriate channel.
type Router struct {
	registry  *Registry
	handler   RouteHandler
	resolver  SessionResolver
	eventBus  *bus.Bus
	mu        sync.RWMutex
	isRunning bool
}

// RouterConfig configures the router.
type RouterConfig struct {
	// Handler processes inbound messages and returns response content.
	Handler RouteHandler
	// SessionResolver maps inbound messages to session IDs.
	// If nil, DefaultSessionResolver is used.
	SessionResolver SessionResolver
}

// NewRouter creates a new message router.
func NewRouter(registry *Registry, eventBus *bus.Bus, cfg RouterConfig) (*Router, error) {
	if cfg.Handler == nil {
		return nil, fmt.Errorf("router handler is required")
	}

	resolver := cfg.SessionResolver
	if resolver == nil {
		resolver = DefaultSessionResolver
	}

	return &Router{
		registry: registry,
		handler:  cfg.Handler,
		resolver: resolver,
		eventBus: eventBus,
	}, nil
}

// DefaultSessionResolver creates a session ID from channel type + sender ID.
// This gives each user a unique session per channel.
func DefaultSessionResolver(msg InboundMessage) string {
	return fmt.Sprintf("%s:%s", msg.ChannelType, msg.SenderID)
}

// Start begins routing messages. It starts all channel receivers with
// the router's inbound handler.
func (r *Router) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.isRunning {
		r.mu.Unlock()
		return fmt.Errorf("router already running")
	}
	r.isRunning = true
	r.mu.Unlock()

	slog.Info("starting channel router")
	return r.registry.StartAll(ctx, r.handleInbound)
}

// Stop stops the router and all channel receivers.
func (r *Router) Stop(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.isRunning {
		return nil
	}

	slog.Info("stopping channel router")
	r.isRunning = false
	return r.registry.StopAll(ctx)
}

// handleInbound is the InboundHandler called by channel receivers.
// It processes each message in its own goroutine.
func (r *Router) handleInbound(ctx context.Context, msg InboundMessage) error {
	// Publish receive event
	if r.eventBus != nil {
		r.eventBus.Publish(ctx, bus.Event{
			Type:    bus.EventMessageReceived,
			Payload: msg,
		})
	}

	// Process in goroutine for concurrency
	go r.processMessage(ctx, msg)
	return nil
}

// processMessage handles a single inbound message end-to-end.
func (r *Router) processMessage(ctx context.Context, msg InboundMessage) {
	sessionID := r.resolver(msg)

	slog.Info("routing message",
		"channel", msg.ChannelType,
		"sender", msg.SenderID,
		"session", sessionID,
	)

	// Invoke the agent handler
	response, err := r.handler(ctx, sessionID, msg.Content)
	if err != nil {
		slog.Error("handler failed",
			"session", sessionID,
			"error", err,
		)
		response = "Sorry, I encountered an error processing your message."
	}

	// Send response back through the channel
	outMsg := OutboundMessage{
		Content:     response,
		ChannelType: msg.ChannelType,
		ChannelName: msg.ChannelName,
		RecipientID: msg.SenderID,
		ThreadID:    msg.ThreadID,
		ReplyToID:   msg.ID,
	}

	sender, ok := r.registry.GetSender(msg.ChannelName)
	if !ok {
		slog.Error("no sender for channel", "name", msg.ChannelName)
		return
	}

	if err := sender.Send(ctx, outMsg); err != nil {
		slog.Error("failed to send response",
			"channel", msg.ChannelName,
			"error", err,
		)
		return
	}

	// Publish send event
	if r.eventBus != nil {
		r.eventBus.Publish(ctx, bus.Event{
			Type:    bus.EventMessageSent,
			Payload: outMsg,
		})
	}

	slog.Info("response sent",
		"channel", msg.ChannelType,
		"recipient", msg.SenderID,
		"session", sessionID,
	)
}
