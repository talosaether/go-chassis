// Package events provides an in-memory publish/subscribe system for the chassis framework.
//
// It enables decoupled communication between modules through event-driven patterns.
// Events are processed synchronously by default, with async publishing available.
//
// # Usage
//
// Register the module with chassis:
//
//	app := chassis.New(
//	    chassis.WithModules(
//	        events.New(),
//	    ),
//	)
//
// Subscribe to events:
//
//	unsubscribe := app.Events().Subscribe("user.created", events.Handler(
//	    func(ctx context.Context, eventType string, payload any) {
//	        user := payload.(*users.User)
//	        log.Printf("New user: %s", user.GetEmail())
//	    },
//	))
//	defer unsubscribe()  // Clean up when done
//
// Publish events:
//
//	// Synchronous - handlers run in sequence
//	app.Events().Publish(ctx, "user.created", user)
//
//	// Asynchronous - handlers run in goroutines
//	app.Events().PublishAsync(ctx, "user.created", user)
//
// # Event Naming
//
// Use dot-separated names following a resource.action pattern:
//
//	user.created, user.updated, user.deleted
//	org.member_added, org.member_removed
//	order.placed, order.shipped, order.completed
//
// # Thread Safety
//
// All operations are thread-safe. Handlers can be registered and events
// published from multiple goroutines.
package events

import (
	"context"
	"sync"

	"github.com/talosaether/chassis"
)

// Handler is a function that handles an event.
type Handler func(ctx context.Context, eventType string, payload any)

// Module is the events module implementation.
// It provides a simple in-memory pub/sub system.
type Module struct {
	mu       sync.RWMutex
	handlers map[string][]Handler
	app      *chassis.App
}

// Option is a function that configures the events module.
type Option func(*Module)

// New creates a new events module with the given options.
func New(opts ...Option) *Module {
	mod := &Module{
		handlers: make(map[string][]Handler),
	}

	for _, opt := range opts {
		opt(mod)
	}

	return mod
}

// Name returns the module identifier.
func (mod *Module) Name() string {
	return "events"
}

// Init initializes the events module.
func (mod *Module) Init(ctx context.Context, app *chassis.App) error {
	mod.app = app
	app.Logger().Info("events module initialized")
	return nil
}

// Shutdown cleans up the events module.
func (mod *Module) Shutdown(ctx context.Context) error {
	mod.mu.Lock()
	defer mod.mu.Unlock()
	mod.handlers = make(map[string][]Handler)
	return nil
}

// Subscribe registers a handler for an event type.
// Returns an unsubscribe function.
func (mod *Module) Subscribe(eventType string, handler any) func() {
	// Type assert to Handler
	handlerFunc, ok := handler.(Handler)
	if !ok {
		// Try function signature
		if fn, ok := handler.(func(context.Context, string, any)); ok {
			handlerFunc = fn
		} else {
			return func() {} // Invalid handler, return no-op unsubscribe
		}
	}
	return mod.subscribe(eventType, handlerFunc)
}

// subscribe is the internal implementation.
func (mod *Module) subscribe(eventType string, handler Handler) func() {
	mod.mu.Lock()
	defer mod.mu.Unlock()

	mod.handlers[eventType] = append(mod.handlers[eventType], handler)

	// Return unsubscribe function
	handlerIndex := len(mod.handlers[eventType]) - 1
	return func() {
		mod.mu.Lock()
		defer mod.mu.Unlock()

		handlers := mod.handlers[eventType]
		if handlerIndex < len(handlers) {
			// Remove handler by setting to nil (preserves indices for other unsubscribes)
			mod.handlers[eventType][handlerIndex] = nil
		}
	}
}

// Publish sends an event to all registered handlers.
// Handlers are called synchronously in the order they were registered.
func (mod *Module) Publish(ctx context.Context, eventType string, payload any) {
	mod.mu.RLock()
	handlers := make([]Handler, len(mod.handlers[eventType]))
	copy(handlers, mod.handlers[eventType])
	mod.mu.RUnlock()

	for _, handler := range handlers {
		if handler != nil {
			handler(ctx, eventType, payload)
		}
	}
}

// PublishAsync sends an event to all registered handlers asynchronously.
// Each handler is called in its own goroutine.
func (mod *Module) PublishAsync(ctx context.Context, eventType string, payload any) {
	mod.mu.RLock()
	handlers := make([]Handler, len(mod.handlers[eventType]))
	copy(handlers, mod.handlers[eventType])
	mod.mu.RUnlock()

	for _, handler := range handlers {
		if handler != nil {
			go handler(ctx, eventType, payload)
		}
	}
}

// HasSubscribers returns true if there are any subscribers for the event type.
func (mod *Module) HasSubscribers(eventType string) bool {
	mod.mu.RLock()
	defer mod.mu.RUnlock()

	handlers, exists := mod.handlers[eventType]
	if !exists {
		return false
	}

	// Check if any non-nil handlers exist
	for _, handler := range handlers {
		if handler != nil {
			return true
		}
	}
	return false
}

// SubscriberCount returns the number of active subscribers for an event type.
func (mod *Module) SubscriberCount(eventType string) int {
	mod.mu.RLock()
	defer mod.mu.RUnlock()

	handlers, exists := mod.handlers[eventType]
	if !exists {
		return 0
	}

	count := 0
	for _, handler := range handlers {
		if handler != nil {
			count++
		}
	}
	return count
}
