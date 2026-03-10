package channel

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Registry manages channel adapter lifecycle and lookup.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]registeredAdapter
	running  bool
}

// registeredAdapter holds a registered adapter and its config.
type registeredAdapter struct {
	adapter  Adapter
	config   ChannelConfig
	receiver Receiver // nil if adapter doesn't implement Receiver
	sender   Sender   // nil if adapter doesn't implement Sender
}

// NewRegistry creates a new empty channel registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]registeredAdapter),
	}
}

// Register adds a channel adapter to the registry.
// The adapter must implement Adapter; it may also implement Receiver, Sender, and/or StreamingSender.
// Returns an error if an adapter with the same name is already registered.
func (r *Registry) Register(adapter Adapter, cfg ChannelConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := cfg.Name
	if name == "" {
		name = string(adapter.Type())
	}

	if _, exists := r.adapters[name]; exists {
		return fmt.Errorf("channel %q already registered", name)
	}

	reg := registeredAdapter{
		adapter: adapter,
		config:  cfg,
	}

	// Detect optional interfaces
	if recv, ok := adapter.(Receiver); ok {
		reg.receiver = recv
	}
	if send, ok := adapter.(Sender); ok {
		reg.sender = send
	}

	r.adapters[name] = reg
	slog.Info("channel registered", "name", name, "type", adapter.Type())
	return nil
}

// Get retrieves a registered adapter by name.
func (r *Registry) Get(name string) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reg, ok := r.adapters[name]
	if !ok {
		return nil, false
	}
	return reg.adapter, true
}

// GetSender retrieves a Sender by channel name.
// Returns nil, false if the channel doesn't exist or doesn't implement Sender.
func (r *Registry) GetSender(name string) (Sender, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	reg, ok := r.adapters[name]
	if !ok || reg.sender == nil {
		return nil, false
	}
	return reg.sender, true
}

// List returns all registered adapter names and their types.
func (r *Registry) List() []ChannelConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()

	configs := make([]ChannelConfig, 0, len(r.adapters))
	for _, reg := range r.adapters {
		configs = append(configs, reg.config)
	}
	return configs
}

// StartAll starts all receivers that have handlers.
// It launches each receiver in a goroutine and returns immediately.
// Returns an error if any receiver fails to start.
func (r *Registry) StartAll(ctx context.Context, handler InboundHandler) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("channel registry already running")
	}

	for name, reg := range r.adapters {
		if reg.receiver == nil {
			continue
		}
		if !reg.config.Enabled {
			slog.Info("channel disabled, skipping", "name", name)
			continue
		}

		slog.Info("starting channel receiver", "name", name, "type", reg.adapter.Type())
		recv := reg.receiver
		go func(n string, rc Receiver) {
			if err := rc.Start(ctx, handler); err != nil {
				slog.Error("channel receiver stopped with error", "name", n, "error", err)
			}
		}(name, recv)
	}

	r.running = true
	return nil
}

// StopAll stops all running receivers gracefully.
func (r *Registry) StopAll(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil
	}

	var firstErr error
	for name, reg := range r.adapters {
		if reg.receiver == nil {
			continue
		}
		slog.Info("stopping channel receiver", "name", name)
		if err := reg.receiver.Stop(ctx); err != nil {
			slog.Error("failed to stop channel", "name", name, "error", err)
			if firstErr == nil {
				firstErr = fmt.Errorf("failed to stop channel %q: %w", name, err)
			}
		}
	}

	r.running = false
	return firstErr
}
