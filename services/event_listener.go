package services

import "sync"

var (
	listenerOnce     sync.Once
	listenerInstance *EventListener
)

// EventListener manages event subscriptions and broadcasting.
type EventListener struct {
	pool map[chan string]struct{}
	sync.RWMutex
}

// GetEventListener returns the singleton instance of EventListener.
func GetEventListener() *EventListener {
	listenerOnce.Do(func() {
		listenerInstance = &EventListener{pool: make(map[chan string]struct{})}
	})
	return listenerInstance
}

// Broadcast sends a message to all subscribers non-blocking.
// If a subscriber's channel is full, the message is skipped for that subscriber.
func (el *EventListener) Broadcast(msg string) {
	el.RLock()
	defer el.RUnlock()

	for ch := range el.pool {
		select {
		case ch <- msg:
		default:
			// Channel full, skip message
		}
	}
}

// Subscribe creates a new subscription channel.
// Returns the channel to receive messages and a cancel function to unsubscribe.
func (el *EventListener) Subscribe(buffer int) (chan string, func()) {
	if buffer <= 0 {
		buffer = 100
	}
	ch := make(chan string, buffer)

	el.Lock()
	el.pool[ch] = struct{}{}
	el.Unlock()

	return ch, func() {
		el.Lock()
		defer el.Unlock()
		if _, ok := el.pool[ch]; ok {
			delete(el.pool, ch)
			close(ch)
		}
	}
}
