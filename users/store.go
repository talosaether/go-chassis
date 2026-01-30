package users

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store defines the interface for user persistence.
// Implement this interface to use a different database backend.
type Store interface {
	Create(ctx context.Context, user *User) error
	GetByID(ctx context.Context, id string) (*User, error)
	GetByEmail(ctx context.Context, email string) (*User, error)
	Update(ctx context.Context, user *User) error
	Delete(ctx context.Context, id string) error
	Close() error
}

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite-backed user store.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Create tables
	if err := initSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func initSchema(db *sql.DB) error {
	schema := `
		CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
	`
	_, err := db.Exec(schema)
	return err
}

// Create inserts a new user into the database.
func (store *SQLiteStore) Create(ctx context.Context, user *User) error {
	query := `INSERT INTO users (id, email, password_hash, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`
	_, err := store.db.ExecContext(ctx, query, user.ID, user.Email, user.PasswordHash, user.CreatedAt, user.UpdatedAt)
	return err
}

// GetByID retrieves a user by their ID.
func (store *SQLiteStore) GetByID(ctx context.Context, id string) (*User, error) {
	query := `SELECT id, email, password_hash, created_at, updated_at FROM users WHERE id = ?`
	row := store.db.QueryRowContext(ctx, query, id)
	return scanUser(row)
}

// GetByEmail retrieves a user by their email address.
func (store *SQLiteStore) GetByEmail(ctx context.Context, email string) (*User, error) {
	query := `SELECT id, email, password_hash, created_at, updated_at FROM users WHERE email = ?`
	row := store.db.QueryRowContext(ctx, query, email)
	return scanUser(row)
}

// Update modifies an existing user in the database.
func (store *SQLiteStore) Update(ctx context.Context, user *User) error {
	query := `UPDATE users SET email = ?, password_hash = ?, updated_at = ? WHERE id = ?`
	result, err := store.db.ExecContext(ctx, query, user.Email, user.PasswordHash, user.UpdatedAt, user.ID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes a user from the database.
func (store *SQLiteStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM users WHERE id = ?`
	result, err := store.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// Close closes the database connection.
func (store *SQLiteStore) Close() error {
	return store.db.Close()
}

func scanUser(row *sql.Row) (*User, error) {
	var user User
	err := row.Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &user, nil
}
