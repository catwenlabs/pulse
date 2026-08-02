package events

import (
	"context"
	"sync"
)

// LibraryChange is an ephemeral invalidation signal. It intentionally carries
// no Story or Entry data; clients reconcile through the HTTP API after
// receiving it.
type LibraryChange struct {
	ID       uint64 `json:"-"`
	SourceID string `json:"source_id,omitempty"`
}

// LibraryChangeHub fans committed library changes out to the in-process HTTP
// clients. It is deliberately not a durable queue: reconnecting clients use
// HTTP reconciliation to recover changes that happened while disconnected.
type LibraryChangeHub struct {
	mu             sync.Mutex
	nextID         uint64
	nextSubscriber uint64
	subscribers    map[uint64]chan LibraryChange
}

func NewLibraryChangeHub() *LibraryChangeHub {
	return &LibraryChangeHub{subscribers: make(map[uint64]chan LibraryChange)}
}

// Subscribe returns a coalescing stream that is closed when ctx is canceled.
// A slow subscriber never blocks a writer; at most the newest pending change
// is retained for that subscriber.
func (hub *LibraryChangeHub) Subscribe(ctx context.Context) <-chan LibraryChange {
	stream := make(chan LibraryChange, 1)
	if ctx.Err() != nil {
		close(stream)
		return stream
	}

	hub.mu.Lock()
	hub.nextSubscriber++
	subscriberID := hub.nextSubscriber
	hub.subscribers[subscriberID] = stream
	hub.mu.Unlock()

	go func() {
		<-ctx.Done()
		hub.mu.Lock()
		if current, ok := hub.subscribers[subscriberID]; ok && current == stream {
			delete(hub.subscribers, subscriberID)
			close(stream)
		}
		hub.mu.Unlock()
	}()
	return stream
}

func (hub *LibraryChangeHub) PublishSource(sourceID string) {
	hub.Publish(LibraryChange{SourceID: sourceID})
}

func (hub *LibraryChangeHub) Publish(change LibraryChange) {
	hub.mu.Lock()
	defer hub.mu.Unlock()

	hub.nextID++
	change.ID = hub.nextID
	for _, stream := range hub.subscribers {
		select {
		case stream <- change:
		default:
			// Replace an older pending signal. The payload is an invalidation,
			// so retaining only the newest one is sufficient.
			select {
			case <-stream:
			default:
			}
			select {
			case stream <- change:
			default:
				// The subscriber may be completing concurrently; its context
				// cleanup will remove it under the same mutex.
			}
		}
	}
}
