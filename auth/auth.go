// Package auth provides authentication and session management for the chassis.
//
// The auth module handles user sessions with secure cookie-based storage.
// It integrates with the users module for authentication.
//
// Basic usage:
//
//	app := chassis.New(
//	    chassis.WithModules(
//	        users.New(),
//	        auth.New(),
//	    ),
//	)
//
//	// Login and create session
//	session, err := app.Auth().Login(ctx, w, "email@example.com", "password")
//
//	// Protect routes with middleware
//	http.Handle("/protected", app.Auth().RequireAuth(handler))
package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/talosaether/chassis"
)

var (
	ErrInvalidSession   = errors.New("invalid or expired session")
	ErrNotAuthenticated = errors.New("not authenticated")
)

// Session represents an authenticated user session.
type Session struct {
	ID        string
	UserID    string
	Token     string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Module is the auth module implementation.
type Module struct {
	store        SessionStore
	dbPath       string
	cookieName   string
	sessionTTL   time.Duration
	secureCookie bool
	app          *chassis.App
}

// Options configures the auth module.
type Options struct {
	Store        SessionStore
	DBPath       string
	CookieName   string
	SessionTTL   time.Duration
	SecureCookie bool
}

// Option is a function that configures the auth module.
type Option func(*Options)

// WithStore sets a custom session store.
func WithStore(store SessionStore) Option {
	return func(opts *Options) {
		opts.Store = store
	}
}

// WithDBPath sets the SQLite database path for sessions.
func WithDBPath(path string) Option {
	return func(opts *Options) {
		opts.DBPath = path
	}
}

// WithCookieName sets the session cookie name.
func WithCookieName(name string) Option {
	return func(opts *Options) {
		opts.CookieName = name
	}
}

// WithSessionTTL sets the session time-to-live.
func WithSessionTTL(ttl time.Duration) Option {
	return func(opts *Options) {
		opts.SessionTTL = ttl
	}
}

// WithSecureCookie sets whether cookies should be secure (HTTPS only).
func WithSecureCookie(secure bool) Option {
	return func(opts *Options) {
		opts.SecureCookie = secure
	}
}

// New creates a new auth module with the given options.
func New(opts ...Option) *Module {
	options := &Options{
		DBPath:       "./data/sessions.db",
		CookieName:   "session",
		SessionTTL:   24 * time.Hour,
		SecureCookie: false,
	}

	for _, opt := range opts {
		opt(options)
	}

	return &Module{
		store:        options.Store,
		dbPath:       options.DBPath,
		cookieName:   options.CookieName,
		sessionTTL:   options.SessionTTL,
		secureCookie: options.SecureCookie,
	}
}

// Name returns the module identifier.
func (mod *Module) Name() string {
	return "auth"
}

// Init initializes the auth module.
func (mod *Module) Init(ctx context.Context, app *chassis.App) error {
	mod.app = app

	// Read config if available
	if cfg := app.ConfigData(); cfg != nil {
		if dbPath := cfg.GetString("auth.db_path"); dbPath != "" {
			mod.dbPath = dbPath
		}
		if cookieName := cfg.GetString("auth.cookie_name"); cookieName != "" {
			mod.cookieName = cookieName
		}
		if ttlStr := cfg.GetString("auth.session_ttl"); ttlStr != "" {
			if ttl, err := time.ParseDuration(ttlStr); err == nil {
				mod.sessionTTL = ttl
			}
		}
		if cfg.GetBool("auth.secure_cookie") {
			mod.secureCookie = true
		}
	}

	// Use custom store if provided, otherwise create SQLite store
	if mod.store == nil {
		sqliteStore, err := NewSQLiteSessionStore(mod.dbPath)
		if err != nil {
			return fmt.Errorf("failed to create session store: %w", err)
		}
		mod.store = sqliteStore
		app.Logger().Info("auth using SQLite session store", "path", mod.dbPath)
	} else {
		app.Logger().Info("auth using custom session store")
	}

	return nil
}

// Shutdown cleans up the auth module.
func (mod *Module) Shutdown(ctx context.Context) error {
	if mod.store != nil {
		return mod.store.Close()
	}
	return nil
}

// UserIdentifier is implemented by user types that can provide their ID.
type UserIdentifier interface {
	GetID() string
}

// Login authenticates a user and creates a session.
// It sets the session cookie on the response writer.
func (mod *Module) Login(ctx context.Context, writer http.ResponseWriter, email, password string) (*Session, error) {
	// Authenticate via users module
	userAny, err := mod.app.Users().Authenticate(ctx, email, password)
	if err != nil {
		return nil, err
	}

	// Extract user ID
	userWithID, ok := userAny.(UserIdentifier)
	if !ok {
		return nil, fmt.Errorf("user type does not implement GetID()")
	}
	userID := userWithID.GetID()

	// Generate session token
	token, err := generateToken(32)
	if err != nil {
		return nil, fmt.Errorf("failed to generate session token: %w", err)
	}

	now := time.Now()
	session := &Session{
		ID:        generateID(),
		UserID:    userID,
		Token:     token,
		ExpiresAt: now.Add(mod.sessionTTL),
		CreatedAt: now,
	}

	if err := mod.store.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Set cookie
	http.SetCookie(writer, &http.Cookie{
		Name:     mod.cookieName,
		Value:    session.Token,
		Path:     "/",
		Expires:  session.ExpiresAt,
		HttpOnly: true,
		Secure:   mod.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})

	return session, nil
}

// Logout invalidates a session and clears the cookie.
func (mod *Module) Logout(ctx context.Context, writer http.ResponseWriter, request *http.Request) error {
	cookie, err := request.Cookie(mod.cookieName)
	if err != nil {
		return nil // No session to logout
	}

	if err := mod.store.DeleteByToken(ctx, cookie.Value); err != nil {
		return err
	}

	// Clear cookie
	http.SetCookie(writer, &http.Cookie{
		Name:     mod.cookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   mod.secureCookie,
		SameSite: http.SameSiteLaxMode,
	})

	return nil
}

// GetSession retrieves the current session from a request.
// Returns ErrInvalidSession if no valid session exists.
func (mod *Module) GetSession(ctx context.Context, request *http.Request) (*Session, error) {
	cookie, err := request.Cookie(mod.cookieName)
	if err != nil {
		return nil, ErrInvalidSession
	}

	session, err := mod.store.GetByToken(ctx, cookie.Value)
	if err != nil {
		return nil, ErrInvalidSession
	}

	if time.Now().After(session.ExpiresAt) {
		_ = mod.store.Delete(ctx, session.ID) // Best-effort cleanup
		return nil, ErrInvalidSession
	}

	return session, nil
}

// GetUserID retrieves the current user ID from a request.
// Returns empty string if not authenticated.
// The request parameter should be *http.Request.
func (mod *Module) GetUserID(ctx context.Context, request any) string {
	httpReq, ok := request.(*http.Request)
	if !ok {
		return ""
	}
	session, err := mod.GetSession(ctx, httpReq)
	if err != nil {
		return ""
	}
	return session.UserID
}

// RequireAuth returns middleware that requires a valid session.
// Responds with 401 Unauthorized if no valid session exists.
func (mod *Module) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		session, err := mod.GetSession(request.Context(), request)
		if err != nil {
			http.Error(writer, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Add session to context
		ctx := context.WithValue(request.Context(), sessionContextKey, session)
		next.ServeHTTP(writer, request.WithContext(ctx))
	})
}

// SessionFromContext retrieves the session from request context.
// Returns nil if no session in context (use after RequireAuth middleware).
func SessionFromContext(ctx context.Context) *Session {
	session, _ := ctx.Value(sessionContextKey).(*Session)
	return session
}

// UserIDFromContext retrieves the user ID from request context.
// Returns empty string if no session in context.
func UserIDFromContext(ctx context.Context) string {
	if session := SessionFromContext(ctx); session != nil {
		return session.UserID
	}
	return ""
}

type contextKey string

const sessionContextKey contextKey = "chassis_session"

func generateToken(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func generateID() string {
	token, _ := generateToken(16)
	return token
}
