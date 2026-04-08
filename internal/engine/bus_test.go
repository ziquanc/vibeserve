package engine

import (
	"sync"
	"testing"
	"time"
)

func TestBusPublishSubscribe(t *testing.T) {
	bus := NewBus()

	var received []Event
	var mu sync.Mutex

	bus.Subscribe(EventRouteAdded, func(e Event) {
		mu.Lock()
		received = append(received, e)
		mu.Unlock()
	})

	bus.Publish(Event{Type: EventRouteAdded, Data: "GET /users"})
	bus.Publish(Event{Type: EventRouteAdded, Data: "POST /users"})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 2 {
		t.Fatalf("expected 2 events, got %d", len(received))
	}
	if received[0].Data != "GET /users" {
		t.Errorf("expected 'GET /users', got %v", received[0].Data)
	}
}

func TestBusFiltersByEventType(t *testing.T) {
	bus := NewBus()

	var count int
	var mu sync.Mutex

	bus.Subscribe(EventRouteAdded, func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	bus.Publish(Event{Type: EventRouteAdded, Data: "match"})
	bus.Publish(Event{Type: EventSchemaAltered, Data: "no match"})
	bus.Publish(Event{Type: EventRouteAdded, Data: "match"})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count != 2 {
		t.Fatalf("expected 2 events (filtered), got %d", count)
	}
}

func TestBusMultipleSubscribers(t *testing.T) {
	bus := NewBus()

	var count1, count2 int
	var mu sync.Mutex

	bus.Subscribe(EventScriptLoaded, func(e Event) {
		mu.Lock()
		count1++
		mu.Unlock()
	})
	bus.Subscribe(EventScriptLoaded, func(e Event) {
		mu.Lock()
		count2++
		mu.Unlock()
	})

	bus.Publish(Event{Type: EventScriptLoaded, Data: "hello"})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if count1 != 1 || count2 != 1 {
		t.Errorf("expected both subscribers to receive event, got %d and %d", count1, count2)
	}
}
