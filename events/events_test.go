package events

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestModule_Name(t *testing.T) {
	mod := New()
	if mod.Name() != "events" {
		t.Errorf("Name() should return 'events', got %q", mod.Name())
	}
}

func TestModule_SubscribeAndPublish(t *testing.T) {
	mod := New()
	ctx := context.Background()

	received := make(chan string, 1)

	mod.Subscribe("test.event", Handler(func(ctx context.Context, eventType string, payload any) {
		received <- payload.(string)
	}))

	mod.Publish(ctx, "test.event", "hello")

	select {
	case msg := <-received:
		if msg != "hello" {
			t.Errorf("received %q, want %q", msg, "hello")
		}
	case <-time.After(time.Second):
		t.Fatal("handler was not called")
	}
}

func TestModule_MultipleHandlers(t *testing.T) {
	mod := New()
	ctx := context.Background()

	var mu sync.Mutex
	var calls []string

	mod.Subscribe("multi.event", Handler(func(ctx context.Context, eventType string, payload any) {
		mu.Lock()
		calls = append(calls, "handler1")
		mu.Unlock()
	}))

	mod.Subscribe("multi.event", Handler(func(ctx context.Context, eventType string, payload any) {
		mu.Lock()
		calls = append(calls, "handler2")
		mu.Unlock()
	}))

	mod.Publish(ctx, "multi.event", nil)

	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 2 {
		t.Errorf("expected 2 handlers to be called, got %d", len(calls))
	}
}

func TestModule_Unsubscribe(t *testing.T) {
	mod := New()
	ctx := context.Background()

	callCount := 0

	unsubscribe := mod.Subscribe("unsub.event", Handler(func(ctx context.Context, eventType string, payload any) {
		callCount++
	}))

	// First publish should trigger handler
	mod.Publish(ctx, "unsub.event", nil)
	if callCount != 1 {
		t.Errorf("expected 1 call, got %d", callCount)
	}

	// Unsubscribe
	unsubscribe()

	// Second publish should not trigger handler
	mod.Publish(ctx, "unsub.event", nil)
	if callCount != 1 {
		t.Errorf("after unsubscribe, expected 1 call, got %d", callCount)
	}
}

func TestModule_PublishNoSubscribers(t *testing.T) {
	mod := New()
	ctx := context.Background()

	// Should not panic when publishing to event with no subscribers
	mod.Publish(ctx, "no.subscribers", "payload")
}

func TestModule_PublishAsync(t *testing.T) {
	mod := New()
	ctx := context.Background()

	done := make(chan bool, 1)

	mod.Subscribe("async.event", Handler(func(ctx context.Context, eventType string, payload any) {
		// Simulate some work
		time.Sleep(10 * time.Millisecond)
		done <- true
	}))

	mod.PublishAsync(ctx, "async.event", nil)

	// Should return immediately (async), then handler completes
	select {
	case <-done:
		// Handler completed successfully
	case <-time.After(time.Second):
		t.Fatal("async handler did not complete")
	}
}

func TestModule_HasSubscribers(t *testing.T) {
	mod := New()

	if mod.HasSubscribers("test.event") {
		t.Error("should have no subscribers initially")
	}

	unsubscribe := mod.Subscribe("test.event", Handler(func(ctx context.Context, eventType string, payload any) {}))

	if !mod.HasSubscribers("test.event") {
		t.Error("should have subscribers after Subscribe")
	}

	unsubscribe()

	if mod.HasSubscribers("test.event") {
		t.Error("should have no subscribers after unsubscribe")
	}
}

func TestModule_SubscriberCount(t *testing.T) {
	mod := New()

	if mod.SubscriberCount("count.event") != 0 {
		t.Error("initial count should be 0")
	}

	unsub1 := mod.Subscribe("count.event", Handler(func(ctx context.Context, eventType string, payload any) {}))
	if mod.SubscriberCount("count.event") != 1 {
		t.Errorf("count should be 1, got %d", mod.SubscriberCount("count.event"))
	}

	unsub2 := mod.Subscribe("count.event", Handler(func(ctx context.Context, eventType string, payload any) {}))
	if mod.SubscriberCount("count.event") != 2 {
		t.Errorf("count should be 2, got %d", mod.SubscriberCount("count.event"))
	}

	unsub1()
	if mod.SubscriberCount("count.event") != 1 {
		t.Errorf("count should be 1 after unsub1, got %d", mod.SubscriberCount("count.event"))
	}

	unsub2()
	if mod.SubscriberCount("count.event") != 0 {
		t.Errorf("count should be 0 after unsub2, got %d", mod.SubscriberCount("count.event"))
	}
}

func TestModule_DifferentEventTypes(t *testing.T) {
	mod := New()
	ctx := context.Background()

	var received1, received2 bool

	mod.Subscribe("event.type1", Handler(func(ctx context.Context, eventType string, payload any) {
		received1 = true
	}))

	mod.Subscribe("event.type2", Handler(func(ctx context.Context, eventType string, payload any) {
		received2 = true
	}))

	mod.Publish(ctx, "event.type1", nil)

	if !received1 {
		t.Error("handler1 should have been called")
	}
	if received2 {
		t.Error("handler2 should not have been called")
	}
}

func TestModule_SubscribeFunctionSignature(t *testing.T) {
	mod := New()
	ctx := context.Background()

	called := false

	// Test that plain function signature also works
	mod.Subscribe("func.event", func(ctx context.Context, eventType string, payload any) {
		called = true
	})

	mod.Publish(ctx, "func.event", nil)

	if !called {
		t.Error("handler with function signature should be called")
	}
}

func TestModule_Shutdown(t *testing.T) {
	mod := New()
	ctx := context.Background()

	mod.Subscribe("shutdown.event", Handler(func(ctx context.Context, eventType string, payload any) {}))

	err := mod.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown should not error, got: %v", err)
	}

	if mod.HasSubscribers("shutdown.event") {
		t.Error("should have no subscribers after Shutdown")
	}
}
