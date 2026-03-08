package bus

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPublishAndReceive(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.Subscribe(EventMessageReceived)
	ctx := context.Background()

	b.Publish(ctx, Event{
		Type:    EventMessageReceived,
		Payload: "hello",
	})

	select {
	case evt := <-sub.C():
		assert.Equal(t, EventMessageReceived, evt.Type)
		assert.Equal(t, "hello", evt.Payload)
		assert.False(t, evt.Time.IsZero())
		assert.Equal(t, ctx, evt.Ctx)
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestMultipleSubscribers(t *testing.T) {
	b := New()
	defer b.Close()

	sub1 := b.Subscribe(EventSessionCreated)
	sub2 := b.Subscribe(EventSessionCreated)

	b.Publish(context.Background(), Event{
		Type:    EventSessionCreated,
		Payload: "session-1",
	})

	for _, sub := range []*Subscription{sub1, sub2} {
		select {
		case evt := <-sub.C():
			assert.Equal(t, EventSessionCreated, evt.Type)
			assert.Equal(t, "session-1", evt.Payload)
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for event")
		}
	}
}

func TestUnsubscribe(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.Subscribe(EventMessageSent)
	sub.Cancel()

	b.Publish(context.Background(), Event{
		Type:    EventMessageSent,
		Payload: "should not receive",
	})

	select {
	case _, ok := <-sub.C():
		assert.False(t, ok, "channel should be closed after cancel")
	case <-time.After(100 * time.Millisecond):
		// Expected: no event received
	}
}

func TestCloseStopsDelivery(t *testing.T) {
	b := New()

	sub := b.Subscribe(EventAgentToolCall)
	b.Close()

	// Channel should be closed after bus close
	_, ok := <-sub.C()
	assert.False(t, ok, "channel should be closed after bus close")
}

func TestPublishAfterCloseIsNoop(t *testing.T) {
	b := New()
	sub := b.Subscribe(EventChannelConnected)
	b.Close()

	// Should not panic
	b.Publish(context.Background(), Event{
		Type:    EventChannelConnected,
		Payload: "should be dropped",
	})

	_, ok := <-sub.C()
	assert.False(t, ok, "channel should be closed")
}

func TestCancelIsIdempotent(t *testing.T) {
	b := New()
	defer b.Close()

	sub := b.Subscribe(EventMessageReceived)

	// Call Cancel multiple times — should not panic
	sub.Cancel()
	sub.Cancel()
	sub.Cancel()
}

func TestConcurrentPublishSubscribe(t *testing.T) {
	b := New()
	defer b.Close()

	const numGoroutines = 10
	const numEvents = 100

	var wg sync.WaitGroup
	subs := make([]*Subscription, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		subs[i] = b.Subscribe(EventMessageReceived)
	}

	// Publish concurrently
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numEvents; j++ {
				b.Publish(context.Background(), Event{
					Type:    EventMessageReceived,
					Payload: j,
				})
			}
		}()
	}

	wg.Wait()

	// Each subscriber should have received events (some may be dropped if buffer full)
	for _, sub := range subs {
		count := 0
		for {
			select {
			case <-sub.C():
				count++
			default:
				goto done
			}
		}
	done:
		assert.Greater(t, count, 0, "subscriber should have received at least one event")
	}
}
