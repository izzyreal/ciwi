package application

import (
	"context"
	"testing"
	"time"
)

func TestChangeHubPublishesInitialResyncAndChanges(t *testing.T) {
	hub := NewChangeHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := hub.Watch(ctx)

	initial := <-changes
	if !initial.Resync || initial.InstanceID == "" || initial.Revision != 0 {
		t.Fatalf("unexpected initial change: %+v", initial)
	}

	published := hub.Publish(ChangeQueue, ChangeQueue, ChangeHistory)
	got := <-changes
	if got.Revision != published.Revision || got.InstanceID != initial.InstanceID || got.Resync {
		t.Fatalf("unexpected published change: %+v", got)
	}
	if len(got.Topics) != 2 || got.Topics[0] != ChangeQueue || got.Topics[1] != ChangeHistory {
		t.Fatalf("unexpected topics: %v", got.Topics)
	}
}

func TestChangeHubSlowSubscriberGetsResync(t *testing.T) {
	hub := NewChangeHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := hub.Watch(ctx)

	// Leave the initial marker unread so the subscriber buffer is full.
	hub.Publish(ChangeProjects)
	got := <-changes
	if !got.Resync || got.Revision != 1 {
		t.Fatalf("expected latest resync marker, got %+v", got)
	}
}

func TestChangeHubClosesSubscription(t *testing.T) {
	hub := NewChangeHub()
	ctx, cancel := context.WithCancel(context.Background())
	changes := hub.Watch(ctx)
	<-changes
	cancel()
	select {
	case _, open := <-changes:
		if open {
			t.Fatal("expected subscription to close")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription did not close")
	}
}
