// Package storage provides file/blob storage capabilities for the chassis.
//
// The storage module supports pluggable providers. By default, it uses
// local filesystem storage, but you can provide your own implementation
// of the Provider interface for S3, GCS, R2, or any other backend.
//
// Basic usage:
//
//	app := chassis.New(
//	    chassis.WithModules(storage.New()),
//	)
//	app.Storage().Put(ctx, "files/doc.pdf", data)
//
// Custom provider:
//
//	app := chassis.New(
//	    chassis.WithModules(
//	        storage.New(storage.WithProvider(myS3Provider)),
//	    ),
//	)
package storage

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/talosaether/chassis"
)

// Provider defines the interface that storage backends must implement.
// Implement this interface to add support for S3, GCS, R2, or any other
// storage system.
type Provider interface {
	// Put stores data at the given key, creating or overwriting as needed.
	Put(ctx context.Context, key string, data []byte) error

	// Get retrieves data for the given key.
	// Returns os.ErrNotExist if the key doesn't exist.
	Get(ctx context.Context, key string) ([]byte, error)

	// Delete removes the data at the given key.
	// Returns nil if the key doesn't exist (idempotent).
	Delete(ctx context.Context, key string) error

	// List returns all keys matching the given prefix.
	// Returns an empty slice if no matches found.
	List(ctx context.Context, prefix string) ([]string, error)
}

// Module is the storage module implementation.
type Module struct {
	provider        Provider
	basePath        string
	basePathFromOpt bool // true if basePath was set via WithBasePath option
}

// Options configures the storage module.
type Options struct {
	Provider        Provider
	BasePath        string // For local provider, the root directory
	BasePathFromOpt bool   // true if BasePath was explicitly set
}

// Option is a function that configures the storage module.
type Option func(*Options)

// WithProvider sets a custom storage provider.
func WithProvider(prov Provider) Option {
	return func(opts *Options) {
		opts.Provider = prov
	}
}

// WithBasePath sets the base path for the local filesystem provider.
func WithBasePath(path string) Option {
	return func(opts *Options) {
		opts.BasePath = path
		opts.BasePathFromOpt = true
	}
}

// New creates a new storage module with the given options.
func New(opts ...Option) *Module {
	options := &Options{
		BasePath: "./data/storage", // Default local path
	}

	for _, opt := range opts {
		opt(options)
	}

	return &Module{
		basePath:        options.BasePath,
		basePathFromOpt: options.BasePathFromOpt,
		provider:        options.Provider,
	}
}

// Name returns the module identifier.
func (mod *Module) Name() string {
	return "storage"
}

// Init initializes the storage module.
func (mod *Module) Init(ctx context.Context, app *chassis.App) error {
	// Read base_path from config if not explicitly set via option
	if !mod.basePathFromOpt {
		if cfg := app.ConfigData(); cfg != nil {
			if configPath := cfg.GetString("storage.base_path"); configPath != "" {
				mod.basePath = configPath
			}
		}
	}

	// If no custom provider, use local filesystem
	if mod.provider == nil {
		// Ensure base directory exists
		if err := os.MkdirAll(mod.basePath, 0750); err != nil {
			return fmt.Errorf("failed to create storage directory: %w", err)
		}
		mod.provider = &LocalProvider{basePath: mod.basePath}
		app.Logger().Info("storage using local filesystem", "path", mod.basePath)
	} else {
		app.Logger().Info("storage using custom provider")
	}

	return nil
}

// Shutdown cleans up the storage module.
func (mod *Module) Shutdown(ctx context.Context) error {
	// Nothing to clean up for basic implementation
	return nil
}

// Put stores data at the given key.
func (mod *Module) Put(ctx context.Context, key string, data []byte) error {
	return mod.provider.Put(ctx, key, data)
}

// Get retrieves data for the given key.
func (mod *Module) Get(ctx context.Context, key string) ([]byte, error) {
	return mod.provider.Get(ctx, key)
}

// Delete removes data at the given key.
func (mod *Module) Delete(ctx context.Context, key string) error {
	return mod.provider.Delete(ctx, key)
}

// List returns all keys matching the prefix.
func (mod *Module) List(ctx context.Context, prefix string) ([]string, error) {
	return mod.provider.List(ctx, prefix)
}

// LocalProvider implements Provider using the local filesystem.
type LocalProvider struct {
	basePath string
}

// Put writes data to a file.
func (local *LocalProvider) Put(ctx context.Context, key string, data []byte) error {
	fullPath := filepath.Join(local.basePath, key)

	// Ensure parent directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(fullPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// Get reads data from a file.
func (local *LocalProvider) Get(ctx context.Context, key string) ([]byte, error) {
	fullPath := filepath.Clean(filepath.Join(local.basePath, key))

	data, err := os.ReadFile(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, os.ErrNotExist
		}
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return data, nil
}

// Delete removes a file.
func (local *LocalProvider) Delete(ctx context.Context, key string) error {
	fullPath := filepath.Join(local.basePath, key)

	err := os.Remove(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// List returns all keys matching the prefix.
func (local *LocalProvider) List(ctx context.Context, prefix string) ([]string, error) {
	var keys []string

	searchPath := filepath.Join(local.basePath, prefix)
	searchDir := filepath.Dir(searchPath)

	err := filepath.WalkDir(searchDir, func(filePath string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		// Get relative path from base
		relPath, err := filepath.Rel(local.basePath, filePath)
		if err != nil {
			return err
		}

		// Check if it matches the prefix
		if strings.HasPrefix(relPath, prefix) {
			keys = append(keys, relPath)
		}

		return nil
	})

	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}

	return keys, nil
}
