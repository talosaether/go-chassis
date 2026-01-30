package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// SessionStore defines the interface for session persistence.
type SessionStore interface {
	Create(ctx context.Context, session *Session) error
	GetByID(ctx context.Context, id string) (*Session, error)
	GetByToken(ctx context.Context, token string) (*Session, error)
	Delete(ctx context.Context, id string) error
	DeleteByToken(ctx context.Context, token string) error
	DeleteByUserID(ctx context.Context, userID string) error
	Close() error
}

// SQLiteSessionStore implements SessionStore using SQLite.
type SQLiteSessionStore struct {
	db *sql.DB
}

// NewSQLiteSessionStore creates a new SQLite-backed session store.
func NewSQLiteSessionStore(dbPath string) (*SQLiteSessionStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := initSessionSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &SQLiteSessionStore{db: db}, nil
}

func initSessionSchema(db *sql.DB) error {
	schema := `
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token TEXT UNIQUE NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
		CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
	`
	_, err := db.Exec(schema)
	return err
}

// Create inserts a new session into the database.
func (store *SQLiteSessionStore) Create(ctx context.Context, session *Session) error {
	query := `INSERT INTO sessions (id, user_id, token, expires_at, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := store.db.ExecContext(ctx, query, session.ID, session.UserID, session.Token, session.ExpiresAt, session.CreatedAt)
	return err
}

// GetByID retrieves a session by its ID.
func (store *SQLiteSessionStore) GetByID(ctx context.Context, id string) (*Session, error) {
	query := `SELECT id, user_id, token, expires_at, created_at FROM sessions WHERE id = ?`
	row := store.db.QueryRowContext(ctx, query, id)
	return scanSession(row)
}

// GetByToken retrieves a session by its token.
func (store *SQLiteSessionStore) GetByToken(ctx context.Context, token string) (*Session, error) {
	query := `SELECT id, user_id, token, expires_at, created_at FROM sessions WHERE token = ?`
	row := store.db.QueryRowContext(ctx, query, token)
	return scanSession(row)
}

// Delete removes a session by its ID.
func (store *SQLiteSessionStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM sessions WHERE id = ?`
	_, err := store.db.ExecContext(ctx, query, id)
	return err
}

// DeleteByToken removes a session by its token.
func (store *SQLiteSessionStore) DeleteByToken(ctx context.Context, token string) error {
	query := `DELETE FROM sessions WHERE token = ?`
	_, err := store.db.ExecContext(ctx, query, token)
	return err
}

// DeleteByUserID removes all sessions for a user.
func (store *SQLiteSessionStore) DeleteByUserID(ctx context.Context, userID string) error {
	query := `DELETE FROM sessions WHERE user_id = ?`
	_, err := store.db.ExecContext(ctx, query, userID)
	return err
}

// Close closes the database connection.
func (store *SQLiteSessionStore) Close() error {
	return store.db.Close()
}

func scanSession(row *sql.Row) (*Session, error) {
	var session Session
	err := row.Scan(&session.ID, &session.UserID, &session.Token, &session.ExpiresAt, &session.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInvalidSession
		}
		return nil, err
	}
	return &session, nil
}
