package events

import (
	"context"
	"testing"
	"time"
)

func TestLibraryChangeHubPublishesAndCoalescesSignals(t *testing.T) {
	hub := NewLibraryChangeHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := hub.Subscribe(ctx)

	hub.PublishSource("source-1")
	hub.PublishSource("source-2")

	change := <-stream
	if change.ID != 2 || change.SourceID != "source-2" {
		t.Fatalf("change = %+v, want latest source-2 signal with id 2", change)
	}
}

func TestLibraryChangeHubClosesSubscriptionOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	stream := NewLibraryChangeHub().Subscribe(ctx)
	cancel()

	select {
	case _, ok := <-stream:
		if ok {
			t.Fatal("subscription returned an event after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription was not closed")
	}
}
