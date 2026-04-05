package controlplane

import (
	"sync"

	"github.com/benjaminwestern/agentic-control/pkg/contract"
)

type EventBus struct {
	mu          sync.RWMutex
	nextID      int
	subscribers map[int]chan contract.RuntimeEvent
}

func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[int]chan contract.RuntimeEvent),
	}
}

func (b *EventBus) Publish(event contract.RuntimeEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, subscriber := range b.subscribers {
		select {
		case subscriber <- event:
		default:
		}
	}
}

func (b *EventBus) Subscribe(buffer int) (<-chan contract.RuntimeEvent, func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.nextID++
	identifier := b.nextID
	channel := make(chan contract.RuntimeEvent, buffer)
	b.subscribers[identifier] = channel

	return channel, func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if subscriber, ok := b.subscribers[identifier]; ok {
			delete(b.subscribers, identifier)
			close(subscriber)
		}
	}
}
