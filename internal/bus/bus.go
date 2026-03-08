package bus

import (
	"context"
	"sync"
	"time"
)

const subscriberBufferSize = 64

// Event represents an event in the system.
type Event struct {
	Type    EventType
	Payload any
	Time    time.Time
	Ctx     context.Context
}

// Subscription represents a subscription to events of a specific type.
type Subscription struct {
	ch       chan Event
	bus      *Bus
	eventTyp EventType
	once     sync.Once
}

// C returns the channel to receive events on.
func (s *Subscription) C() <-chan Event {
	return s.ch
}

// Cancel unsubscribes from the event bus. It is safe to call multiple times.
func (s *Subscription) Cancel() {
	s.once.Do(func() {
		s.bus.removeSub(s)
	})
}

// Bus is a simple in-process event bus using Go channels.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[EventType][]*Subscription
	closed      bool
}

// New creates a new event bus.
func New() *Bus {
	return &Bus{
		subscribers: make(map[EventType][]*Subscription),
	}
}

// Publish sends an event to all subscribers of the event type.
// If the bus is closed, it is a no-op.
// If a subscriber's channel is full, the event is dropped for that subscriber.
func (b *Bus) Publish(ctx context.Context, evt Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	evt.Ctx = ctx
	if evt.Time.IsZero() {
		evt.Time = time.Now()
	}

	for _, sub := range b.subscribers[evt.Type] {
		select {
		case sub.ch <- evt:
		default:
			// Drop event if subscriber channel is full.
		}
	}
}

// Subscribe creates a subscription for the given event type.
// Panics if the bus is closed.
func (b *Bus) Subscribe(t EventType) *Subscription {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		panic("bus: subscribe on closed bus")
	}

	sub := &Subscription{
		ch:       make(chan Event, subscriberBufferSize),
		bus:      b,
		eventTyp: t,
	}
	b.subscribers[t] = append(b.subscribers[t], sub)
	return sub
}

// Close shuts down the bus, closing all subscriber channels and clearing subscriptions.
func (b *Bus) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true
	for _, subs := range b.subscribers {
		for _, sub := range subs {
			close(sub.ch)
		}
	}
	b.subscribers = make(map[EventType][]*Subscription)
	return nil
}

// removeSub removes a subscription from the bus.
func (b *Bus) removeSub(sub *Subscription) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	subs := b.subscribers[sub.eventTyp]
	for i, s := range subs {
		if s == sub {
			b.subscribers[sub.eventTyp] = append(subs[:i], subs[i+1:]...)
			close(sub.ch)
			return
		}
	}
}
