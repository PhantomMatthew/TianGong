package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/PhantomMatthew/TianGong/internal/channel"
)

func TestAdapterTypeAndName(t *testing.T) {
	a := New()
	assert.Equal(t, channel.TypeCLI, a.Type())
	assert.Equal(t, "cli", a.Name())
}

func TestAdapterCustomName(t *testing.T) {
	a := New(WithName("dev-cli"))
	assert.Equal(t, "dev-cli", a.Name())
}

func TestAdapterImplementsInterfaces(t *testing.T) {
	a := New()
	var _ channel.Adapter = a
	var _ channel.Receiver = a
	var _ channel.Sender = a
}

func TestSend(t *testing.T) {
	var buf bytes.Buffer
	a := New(WithOutput(&buf))

	err := a.Send(context.Background(), channel.OutboundMessage{
		Content: "hello world",
	})
	require.NoError(t, err)
	assert.Equal(t, "hello world\n", buf.String())
}

func TestSendMultipleMessages(t *testing.T) {
	var buf bytes.Buffer
	a := New(WithOutput(&buf))

	ctx := context.Background()
	require.NoError(t, a.Send(ctx, channel.OutboundMessage{Content: "first"}))
	require.NoError(t, a.Send(ctx, channel.OutboundMessage{Content: "second"}))

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	assert.Equal(t, []string{"first", "second"}, lines)
}

func TestStartReadsInput(t *testing.T) {
	input := strings.NewReader("hello\nworld\n")
	var output bytes.Buffer

	a := New(
		WithInput(input),
		WithOutput(&output),
		WithSenderID("test-user"),
		WithPrompt("$ "),
	)

	var mu sync.Mutex
	var received []string

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg.Content)
		mu.Unlock()
		assert.Equal(t, channel.TypeCLI, msg.ChannelType)
		assert.Equal(t, "cli", msg.ChannelName)
		assert.Equal(t, "test-user", msg.SenderID)
		assert.NotEmpty(t, msg.ID)
		assert.False(t, msg.Timestamp.IsZero())
		return nil
	}

	err := a.Start(context.Background(), handler)
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, []string{"hello", "world"}, received)
	mu.Unlock()

	// Verify prompts were written.
	assert.Contains(t, output.String(), "$ ")
}

func TestStartSkipsEmptyLines(t *testing.T) {
	input := strings.NewReader("\n  \nhello\n\n")
	var output bytes.Buffer

	a := New(WithInput(input), WithOutput(&output))

	var mu sync.Mutex
	var received []string

	handler := func(_ context.Context, msg channel.InboundMessage) error {
		mu.Lock()
		received = append(received, msg.Content)
		mu.Unlock()
		return nil
	}

	err := a.Start(context.Background(), handler)
	require.NoError(t, err)

	mu.Lock()
	assert.Equal(t, []string{"hello"}, received)
	mu.Unlock()
}

func TestStartStopsOnContextCancel(t *testing.T) {
	// Use a pipe that never closes to simulate blocking input.
	pr, pw := syncPipe()
	defer pw.Close()
	var output bytes.Buffer

	a := New(WithInput(pr), WithOutput(&output))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Start(ctx, handler)
	}()

	// Give Start time to begin.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}

func TestStartStopsOnStop(t *testing.T) {
	pr, pw := syncPipe()
	defer pw.Close()
	var output bytes.Buffer

	a := New(WithInput(pr), WithOutput(&output))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- a.Start(context.Background(), handler)
	}()

	time.Sleep(50 * time.Millisecond)

	err := a.Stop(context.Background())
	require.NoError(t, err)

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after Stop")
	}
}

func TestDoubleStartReturnsError(t *testing.T) {
	pr, pw := syncPipe()
	defer pw.Close()
	var output bytes.Buffer

	a := New(WithInput(pr), WithOutput(&output))

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}

	go func() {
		_ = a.Start(context.Background(), handler)
	}()

	time.Sleep(50 * time.Millisecond)

	err := a.Start(context.Background(), handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already running")

	_ = a.Stop(context.Background())
}

func TestStopWhenNotRunningIsNoop(t *testing.T) {
	a := New()
	err := a.Stop(context.Background())
	assert.NoError(t, err)
}

func TestCustomPrompt(t *testing.T) {
	input := strings.NewReader("hi\n")
	var output bytes.Buffer

	a := New(
		WithInput(input),
		WithOutput(&output),
		WithPrompt("tiangong> "),
	)

	handler := func(_ context.Context, _ channel.InboundMessage) error {
		return nil
	}

	err := a.Start(context.Background(), handler)
	require.NoError(t, err)

	assert.Contains(t, output.String(), "tiangong> ")
}

// syncPipe creates an io.Reader/io.Writer pair backed by a bytes.Buffer
// with a channel-based blocking mechanism suitable for testing.
func syncPipe() (*pipeReader, *pipeWriter) {
	ch := make(chan byte, 4096)
	return &pipeReader{ch: ch}, &pipeWriter{ch: ch}
}

type pipeReader struct {
	ch chan byte
}

func (r *pipeReader) Read(p []byte) (int, error) {
	b, ok := <-r.ch
	if !ok {
		return 0, io.EOF
	}
	p[0] = b
	return 1, nil
}

type pipeWriter struct {
	ch chan byte
}

func (w *pipeWriter) Write(p []byte) (int, error) {
	for _, b := range p {
		w.ch <- b
	}
	return len(p), nil
}

func (w *pipeWriter) Close() error {
	close(w.ch)
	return nil
}
