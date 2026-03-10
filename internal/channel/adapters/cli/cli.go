// Package cli provides a CLI channel adapter for interactive terminal sessions.
//
// The CLI adapter reads messages from an io.Reader (typically os.Stdin) and
// writes responses to an io.Writer (typically os.Stdout). It implements
// Adapter, Receiver, and Sender from the channel package.
//
// This adapter is primarily used for development, testing, and the `tg chat`
// command where the agent communicates directly through the terminal.
package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PhantomMatthew/TianGong/internal/channel"
)

const (
	// DefaultPrompt is the default input prompt displayed to the user.
	DefaultPrompt = "> "
	// DefaultSenderID is the default sender ID for CLI messages.
	DefaultSenderID = "cli-user"
)

// Option configures the CLI adapter.
type Option func(*Adapter)

// WithInput sets the input reader (default: os.Stdin).
func WithInput(r io.Reader) Option {
	return func(a *Adapter) {
		a.input = r
	}
}

// WithOutput sets the output writer (default: os.Stdout).
func WithOutput(w io.Writer) Option {
	return func(a *Adapter) {
		a.output = w
	}
}

// WithPrompt sets the input prompt string (default: "> ").
func WithPrompt(prompt string) Option {
	return func(a *Adapter) {
		a.prompt = prompt
	}
}

// WithSenderID sets the sender ID for inbound messages (default: "cli-user").
func WithSenderID(id string) Option {
	return func(a *Adapter) {
		a.senderID = id
	}
}

// WithName sets the adapter instance name (default: "cli").
func WithName(name string) Option {
	return func(a *Adapter) {
		a.name = name
	}
}

// Adapter is a CLI channel adapter for interactive terminal sessions.
// It reads lines from an input reader and writes responses to an output writer.
type Adapter struct {
	name     string
	input    io.Reader
	output   io.Writer
	prompt   string
	senderID string

	mu      sync.Mutex
	running bool
	stopCh  chan struct{}
}

// New creates a new CLI adapter with the given options.
func New(opts ...Option) *Adapter {
	a := &Adapter{
		name:     "cli",
		input:    os.Stdin,
		output:   os.Stdout,
		prompt:   DefaultPrompt,
		senderID: DefaultSenderID,
		stopCh:   make(chan struct{}),
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Type returns the channel type identifier.
func (a *Adapter) Type() channel.ChannelType {
	return channel.TypeCLI
}

// Name returns the adapter instance name.
func (a *Adapter) Name() string {
	return a.name
}

// Start begins reading lines from input and dispatching them via the handler.
// It blocks until the context is cancelled, Stop is called, or input reaches EOF.
func (a *Adapter) Start(ctx context.Context, handler channel.InboundHandler) error {
	a.mu.Lock()
	if a.running {
		a.mu.Unlock()
		return fmt.Errorf("cli adapter already running")
	}
	a.running = true
	a.mu.Unlock()

	slog.Info("cli adapter started", "name", a.name)

	scanner := bufio.NewScanner(a.input)

	// readCh signals when a line is available or input ends.
	type scanResult struct {
		text string
		ok   bool
	}
	readCh := make(chan scanResult)

	go func() {
		defer close(readCh)
		for scanner.Scan() {
			readCh <- scanResult{text: scanner.Text(), ok: true}
		}
		// EOF or error — signal done.
		readCh <- scanResult{ok: false}
	}()

	a.writePrompt()

	for {
		select {
		case <-ctx.Done():
			a.setRunning(false)
			return ctx.Err()
		case <-a.stopCh:
			a.setRunning(false)
			return nil
		case result, chOpen := <-readCh:
			if !chOpen || !result.ok {
				a.setRunning(false)
				return nil
			}

			text := strings.TrimSpace(result.text)
			if text == "" {
				a.writePrompt()
				continue
			}

			msg := channel.InboundMessage{
				ID:          fmt.Sprintf("cli-%d", time.Now().UnixNano()),
				ChannelType: channel.TypeCLI,
				ChannelName: a.name,
				SenderID:    a.senderID,
				SenderName:  a.senderID,
				Content:     text,
				Timestamp:   time.Now(),
			}

			if err := handler(ctx, msg); err != nil {
				slog.Error("cli handler error", "error", err)
			}

			a.writePrompt()
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
	slog.Info("cli adapter stopped", "name", a.name)
	return nil
}

// Send writes an outbound message to the output writer.
func (a *Adapter) Send(_ context.Context, msg channel.OutboundMessage) error {
	_, err := fmt.Fprintln(a.output, msg.Content)
	return err
}

// writePrompt writes the prompt string to output.
func (a *Adapter) writePrompt() {
	fmt.Fprint(a.output, a.prompt)
}

// setRunning safely sets the running flag.
func (a *Adapter) setRunning(v bool) {
	a.mu.Lock()
	a.running = v
	a.mu.Unlock()
}
