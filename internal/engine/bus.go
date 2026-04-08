package engine

import "sync"

type SubscriberFunc func(Event)

type subscriber struct {
	fn   SubscriberFunc
	ch   chan Event
	done chan struct{}
}

func newSubscriber(fn SubscriberFunc) *subscriber {
	s := &subscriber{
		fn:   fn,
		ch:   make(chan Event, 64),
		done: make(chan struct{}),
	}
	go func() {
		defer close(s.done)
		for e := range s.ch {
			s.fn(e)
		}
	}()
	return s
}

type Bus struct {
	mu   sync.RWMutex
	subs map[EventType][]*subscriber
}

func NewBus() *Bus {
	return &Bus{subs: make(map[EventType][]*subscriber)}
}

func (b *Bus) Subscribe(t EventType, fn SubscriberFunc) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[t] = append(b.subs[t], newSubscriber(fn))
}

func (b *Bus) Publish(e Event) {
	b.mu.RLock()
	subs := b.subs[e.Type]
	b.mu.RUnlock()

	for _, s := range subs {
		s.ch <- e
	}
}
