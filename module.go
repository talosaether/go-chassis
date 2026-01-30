package chassis

import "context"

// Module is the interface that all chassis modules must implement.
// It provides lifecycle hooks and identification.
type Module interface {
	// Name returns a unique identifier for this module
	Name() string

	// Init is called when the module is registered with the chassis.
	// Use this for setup, connection pooling, migrations, etc.
	Init(ctx context.Context, app *App) error

	// Shutdown is called when the chassis is stopping.
	// Use this for cleanup, closing connections, flushing buffers, etc.
	Shutdown(ctx context.Context) error
}

// ModuleOption is a function that configures a module during creation.
// Each module defines its own option functions.
type ModuleOption func(interface{})
