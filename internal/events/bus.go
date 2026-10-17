// Package events is a small non-blocking fan-out bus. The core service
// publishes onto it; the desktop shell and the browser SSE endpoint relay
// events to the UI.
package events

import (
	"sync"
	"time"
)

// Event is one thing that happened.
type Event struct {
	Type string    `json:"type"`
	Time time.Time `json:"time"`
	Data any       `json:"data,omitempty"`
}

// Bus fans events out to subscribers. Publishing never blocks: a subscriber
// whose buffer is full misses the event rather than stalling the service.
type Bus struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// NewBus returns an empty bus.
func NewBus() *Bus { return &Bus{subs: map[chan Event]struct{}{}} }

// Subscribe returns a channel of events and a cancel func that removes the
// subscription and closes the channel.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 256)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
}

// Publish sends an event to every subscriber, dropping it for any whose
// buffer is full.
func (b *Bus) Publish(typ string, data any) {
	ev := Event{Type: typ, Time: time.Now(), Data: data}
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- ev:
		default:
		}
	}
}
