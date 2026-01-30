// Package cache provides key-value caching for the chassis framework.
//
// It supports time-based expiration (TTL) and pluggable storage backends.
// The default implementation uses an in-memory store with automatic cleanup.
//
// # Usage
//
// Register the module with chassis:
//
//	app := chassis.New(
//	    chassis.WithModules(
//	        cache.New(),
//	    ),
//	)
//
// Store and retrieve values:
//
//	// Set with default TTL (5 minutes)
//	app.Cache().Set(ctx, "user:123", []byte(`{"name":"Alice"}`))
//
//	// Set with custom TTL
//	app.Cache().SetWithTTL(ctx, "session:abc", data, time.Hour)
//
//	// Get value
//	data, found := app.Cache().Get(ctx, "user:123")
//	if found {
//	    // Use cached data
//	}
//
//	// Delete value
//	app.Cache().Delete(ctx, "user:123")
//
// # Configuration
//
// Configure via config.yaml:
//
//	cache:
//	  default_ttl: 10m
//
// Or programmatically:
//
//	cache.New(cache.WithDefaultTTL(10 * time.Minute))
//
// # Custom Providers
//
// Implement the Provider interface for custom backends (e.g., Redis):
//
//	cache.New(cache.WithProvider(myRedisProvider))
package cache

import (
	"context"
	"sync"
	"time"

	"github.com/talosaether/chassis"
)

// Provider defines the interface for cache implementations.
type Provider interface {
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
}

// Module is the cache module implementation.
type Module struct {
	provider   Provider
	defaultTTL time.Duration
	app        *chassis.App
}

// Option is a function that configures the cache module.
type Option func(*Module)

// WithProvider sets a custom cache provider.
func WithProvider(provider Provider) Option {
	return func(mod *Module) {
		mod.provider = provider
	}
}

// WithDefaultTTL sets the default TTL for cache entries.
func WithDefaultTTL(ttl time.Duration) Option {
	return func(mod *Module) {
		mod.defaultTTL = ttl
	}
}

// New creates a new cache module with the given options.
func New(opts ...Option) *Module {
	mod := &Module{
		defaultTTL: 5 * time.Minute,
	}

	for _, opt := range opts {
		opt(mod)
	}

	return mod
}

// Name returns the module identifier.
func (mod *Module) Name() string {
	return "cache"
}

// Init initializes the cache module.
func (mod *Module) Init(ctx context.Context, app *chassis.App) error {
	mod.app = app

	// Read config if available
	if cfg := app.ConfigData(); cfg != nil {
		if ttlStr := cfg.GetString("cache.default_ttl"); ttlStr != "" {
			if ttl, err := time.ParseDuration(ttlStr); err == nil {
				mod.defaultTTL = ttl
			}
		}
	}

	// Use default in-memory provider if none provided
	if mod.provider == nil {
		mod.provider = NewMemoryProvider()
	}

	app.Logger().Info("cache module initialized", "default_ttl", mod.defaultTTL)
	return nil
}

// Shutdown cleans up the cache module.
func (mod *Module) Shutdown(ctx context.Context) error {
	return mod.provider.Clear(ctx)
}

// Get retrieves a value from the cache.
func (mod *Module) Get(ctx context.Context, key string) ([]byte, bool) {
	return mod.provider.Get(ctx, key)
}

// Set stores a value in the cache with the default TTL.
func (mod *Module) Set(ctx context.Context, key string, value []byte) error {
	return mod.provider.Set(ctx, key, value, mod.defaultTTL)
}

// SetWithTTL stores a value in the cache with a custom TTL.
func (mod *Module) SetWithTTL(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return mod.provider.Set(ctx, key, value, ttl)
}

// Delete removes a value from the cache.
func (mod *Module) Delete(ctx context.Context, key string) error {
	return mod.provider.Delete(ctx, key)
}

// Clear removes all values from the cache.
func (mod *Module) Clear(ctx context.Context) error {
	return mod.provider.Clear(ctx)
}

// MemoryProvider is an in-memory cache implementation.
type MemoryProvider struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
}

type cacheEntry struct {
	value     []byte
	expiresAt time.Time
}

// NewMemoryProvider creates a new in-memory cache provider.
func NewMemoryProvider() *MemoryProvider {
	provider := &MemoryProvider{
		entries: make(map[string]*cacheEntry),
	}

	// Start background cleanup goroutine
	go provider.cleanup()

	return provider
}

func (provider *MemoryProvider) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		provider.mu.Lock()
		now := time.Now()
		for key, entry := range provider.entries {
			if now.After(entry.expiresAt) {
				delete(provider.entries, key)
			}
		}
		provider.mu.Unlock()
	}
}

func (provider *MemoryProvider) Get(ctx context.Context, key string) ([]byte, bool) {
	provider.mu.RLock()
	defer provider.mu.RUnlock()

	entry, exists := provider.entries[key]
	if !exists {
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		return nil, false
	}

	return entry.value, true
}

func (provider *MemoryProvider) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	provider.mu.Lock()
	defer provider.mu.Unlock()

	provider.entries[key] = &cacheEntry{
		value:     value,
		expiresAt: time.Now().Add(ttl),
	}
	return nil
}

func (provider *MemoryProvider) Delete(ctx context.Context, key string) error {
	provider.mu.Lock()
	defer provider.mu.Unlock()

	delete(provider.entries, key)
	return nil
}

func (provider *MemoryProvider) Clear(ctx context.Context) error {
	provider.mu.Lock()
	defer provider.mu.Unlock()

	provider.entries = make(map[string]*cacheEntry)
	return nil
}
