// Package users provides user management capabilities for the chassis.
//
// The users module handles user creation, authentication, and profile management.
// By default it uses SQLite for storage, but you can provide your own Store implementation.
//
// Basic usage:
//
//	app := chassis.New(
//	    chassis.WithModules(users.New()),
//	)
//	user, err := app.Users().Create(ctx, users.CreateInput{
//	    Email: "test@example.com",
//	    Password: "secret",
//	})
package users

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/talosaether/chassis"
	"golang.org/x/crypto/argon2"
)

var (
	ErrNotFound      = errors.New("user not found")
	ErrEmailExists   = errors.New("email already exists")
	ErrInvalidEmail  = errors.New("invalid email")
	ErrWeakPassword  = errors.New("password too weak (minimum 8 characters)")
	ErrWrongPassword = errors.New("wrong password")
)

// User represents a user in the system.
type User struct {
	ID           string
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// GetID returns the user's ID.
func (user *User) GetID() string {
	return user.ID
}

// GetEmail returns the user's email.
func (user *User) GetEmail() string {
	return user.Email
}

// UpdateInput contains the data that can be updated on a user.
type UpdateInput struct {
	Email    *string
	Password *string
}

// Module is the users module implementation.
type Module struct {
	store  Store
	dbPath string
	app    *chassis.App
}

// Options configures the users module.
type Options struct {
	Store  Store
	DBPath string
}

// Option is a function that configures the users module.
type Option func(*Options)

// WithStore sets a custom store implementation.
func WithStore(store Store) Option {
	return func(opts *Options) {
		opts.Store = store
	}
}

// WithDBPath sets the SQLite database path.
func WithDBPath(path string) Option {
	return func(opts *Options) {
		opts.DBPath = path
	}
}

// New creates a new users module with the given options.
func New(opts ...Option) *Module {
	options := &Options{
		DBPath: "./data/users.db",
	}

	for _, opt := range opts {
		opt(options)
	}

	return &Module{
		store:  options.Store,
		dbPath: options.DBPath,
	}
}

// Name returns the module identifier.
func (mod *Module) Name() string {
	return "users"
}

// Init initializes the users module.
func (mod *Module) Init(ctx context.Context, app *chassis.App) error {
	mod.app = app

	// Read config if available
	if cfg := app.ConfigData(); cfg != nil {
		if dbPath := cfg.GetString("users.db_path"); dbPath != "" {
			mod.dbPath = dbPath
		}
	}

	// Use custom store if provided, otherwise create SQLite store
	if mod.store == nil {
		sqliteStore, err := NewSQLiteStore(mod.dbPath)
		if err != nil {
			return fmt.Errorf("failed to create users store: %w", err)
		}
		mod.store = sqliteStore
		app.Logger().Info("users using SQLite store", "path", mod.dbPath)
	} else {
		app.Logger().Info("users using custom store")
	}

	return nil
}

// Shutdown cleans up the users module.
func (mod *Module) Shutdown(ctx context.Context) error {
	if mod.store != nil {
		return mod.store.Close()
	}
	return nil
}

// Create creates a new user with the given email and password.
func (mod *Module) Create(ctx context.Context, email, password string) (any, error) {
	if email == "" {
		return nil, ErrInvalidEmail
	}
	if len(password) < 8 {
		return nil, ErrWeakPassword
	}

	// Check if email already exists
	existing, err := mod.store.GetByEmail(ctx, email)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("failed to check existing user: %w", err)
	}
	if existing != nil {
		return nil, ErrEmailExists
	}

	// Hash password
	hash, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	now := time.Now()
	user := &User{
		ID:           uuid.New().String(),
		Email:        email,
		PasswordHash: hash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	if err := mod.store.Create(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// GetByID retrieves a user by their ID.
func (mod *Module) GetByID(ctx context.Context, id string) (any, error) {
	return mod.store.GetByID(ctx, id)
}

// GetByEmail retrieves a user by their email.
func (mod *Module) GetByEmail(ctx context.Context, email string) (any, error) {
	return mod.store.GetByEmail(ctx, email)
}

// Update updates an existing user.
func (mod *Module) Update(ctx context.Context, id string, input UpdateInput) (*User, error) {
	user, err := mod.store.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if input.Email != nil {
		if *input.Email == "" {
			return nil, ErrInvalidEmail
		}
		// Check if new email already exists for a different user
		existing, err := mod.store.GetByEmail(ctx, *input.Email)
		if err != nil && !errors.Is(err, ErrNotFound) {
			return nil, fmt.Errorf("failed to check existing user: %w", err)
		}
		if existing != nil && existing.ID != id {
			return nil, ErrEmailExists
		}
		user.Email = *input.Email
	}

	if input.Password != nil {
		if len(*input.Password) < 8 {
			return nil, ErrWeakPassword
		}
		hash, err := hashPassword(*input.Password)
		if err != nil {
			return nil, fmt.Errorf("failed to hash password: %w", err)
		}
		user.PasswordHash = hash
	}

	user.UpdatedAt = time.Now()

	if err := mod.store.Update(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to update user: %w", err)
	}

	return user, nil
}

// Delete removes a user by their ID.
func (mod *Module) Delete(ctx context.Context, id string) error {
	return mod.store.Delete(ctx, id)
}

// Authenticate verifies a user's email and password, returning the user if valid.
func (mod *Module) Authenticate(ctx context.Context, email, password string) (any, error) {
	user, err := mod.store.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, ErrWrongPassword // Don't reveal if email exists
		}
		return nil, err
	}

	if !verifyPassword(password, user.PasswordHash) {
		return nil, ErrWrongPassword
	}

	return user, nil
}

// Password hashing using Argon2id
const (
	argonTime    = 1
	argonMemory  = 64 * 1024
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
)

func hashPassword(password string) (string, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}

	hash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	// Encode as: salt$hash (both base64)
	saltB64 := base64.RawStdEncoding.EncodeToString(salt)
	hashB64 := base64.RawStdEncoding.EncodeToString(hash)

	return saltB64 + "$" + hashB64, nil
}

func verifyPassword(password, encoded string) bool {
	// Split salt$hash
	var saltB64, hashB64 string
	for i, c := range encoded {
		if c == '$' {
			saltB64 = encoded[:i]
			hashB64 = encoded[i+1:]
			break
		}
	}
	if saltB64 == "" || hashB64 == "" {
		return false
	}

	salt, err := base64.RawStdEncoding.DecodeString(saltB64)
	if err != nil {
		return false
	}

	expectedHash, err := base64.RawStdEncoding.DecodeString(hashB64)
	if err != nil {
		return false
	}

	actualHash := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)

	// Constant-time comparison
	if len(actualHash) != len(expectedHash) {
		return false
	}
	var diff byte
	for i := range actualHash {
		diff |= actualHash[i] ^ expectedHash[i]
	}
	return diff == 0
}
