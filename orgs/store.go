package orgs

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// Store defines the interface for organization persistence.
type Store interface {
	Create(ctx context.Context, org *Org) error
	GetByID(ctx context.Context, id string) (*Org, error)
	GetByName(ctx context.Context, name string) (*Org, error)
	Update(ctx context.Context, org *Org) error
	Delete(ctx context.Context, id string) error

	CreateMembership(ctx context.Context, membership *Membership) error
	GetMembership(ctx context.Context, orgID, userID string) (*Membership, error)
	GetMembersByOrgID(ctx context.Context, orgID string) ([]*Membership, error)
	GetMembershipsByUserID(ctx context.Context, userID string) ([]*Membership, error)
	UpdateMembership(ctx context.Context, membership *Membership) error
	DeleteMembership(ctx context.Context, orgID, userID string) error
	DeleteMembershipsByOrgID(ctx context.Context, orgID string) error

	Close() error
}

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite-backed organization store.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := initOrgSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func initOrgSchema(db *sql.DB) error {
	schema := `
		CREATE TABLE IF NOT EXISTS orgs (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL UNIQUE,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);

		CREATE TABLE IF NOT EXISTS memberships (
			id TEXT PRIMARY KEY,
			org_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			UNIQUE(org_id, user_id)
		);
		CREATE INDEX IF NOT EXISTS idx_memberships_org_id ON memberships(org_id);
		CREATE INDEX IF NOT EXISTS idx_memberships_user_id ON memberships(user_id);
	`
	_, err := db.Exec(schema)
	return err
}

func (store *SQLiteStore) Create(ctx context.Context, org *Org) error {
	query := `INSERT INTO orgs (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)`
	_, err := store.db.ExecContext(ctx, query, org.id, org.Name, org.CreatedAt, org.UpdatedAt)
	return err
}

func (store *SQLiteStore) GetByID(ctx context.Context, id string) (*Org, error) {
	query := `SELECT id, name, created_at, updated_at FROM orgs WHERE id = ?`
	row := store.db.QueryRowContext(ctx, query, id)

	var org Org
	err := row.Scan(&org.id, &org.Name, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &org, nil
}

func (store *SQLiteStore) GetByName(ctx context.Context, name string) (*Org, error) {
	query := `SELECT id, name, created_at, updated_at FROM orgs WHERE name = ?`
	row := store.db.QueryRowContext(ctx, query, name)

	var org Org
	err := row.Scan(&org.id, &org.Name, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &org, nil
}

func (store *SQLiteStore) Update(ctx context.Context, org *Org) error {
	query := `UPDATE orgs SET name = ?, updated_at = ? WHERE id = ?`
	result, err := store.db.ExecContext(ctx, query, org.Name, org.UpdatedAt, org.id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (store *SQLiteStore) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM orgs WHERE id = ?`
	result, err := store.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (store *SQLiteStore) CreateMembership(ctx context.Context, membership *Membership) error {
	query := `INSERT INTO memberships (id, org_id, user_id, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`
	_, err := store.db.ExecContext(ctx, query, membership.ID, membership.OrgID, membership.UserID, membership.Role, membership.CreatedAt, membership.UpdatedAt)
	return err
}

func (store *SQLiteStore) GetMembership(ctx context.Context, orgID, userID string) (*Membership, error) {
	query := `SELECT id, org_id, user_id, role, created_at, updated_at FROM memberships WHERE org_id = ? AND user_id = ?`
	row := store.db.QueryRowContext(ctx, query, orgID, userID)

	var membership Membership
	err := row.Scan(&membership.ID, &membership.OrgID, &membership.UserID, &membership.Role, &membership.CreatedAt, &membership.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMemberNotFound
		}
		return nil, err
	}
	return &membership, nil
}

func (store *SQLiteStore) GetMembersByOrgID(ctx context.Context, orgID string) ([]*Membership, error) {
	query := `SELECT id, org_id, user_id, role, created_at, updated_at FROM memberships WHERE org_id = ?`
	rows, err := store.db.QueryContext(ctx, query, orgID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var memberships []*Membership
	for rows.Next() {
		membership := &Membership{}
		err := rows.Scan(&membership.ID, &membership.OrgID, &membership.UserID, &membership.Role, &membership.CreatedAt, &membership.UpdatedAt)
		if err != nil {
			return nil, err
		}
		memberships = append(memberships, membership)
	}
	return memberships, rows.Err()
}

func (store *SQLiteStore) GetMembershipsByUserID(ctx context.Context, userID string) ([]*Membership, error) {
	query := `SELECT id, org_id, user_id, role, created_at, updated_at FROM memberships WHERE user_id = ?`
	rows, err := store.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var memberships []*Membership
	for rows.Next() {
		membership := &Membership{}
		err := rows.Scan(&membership.ID, &membership.OrgID, &membership.UserID, &membership.Role, &membership.CreatedAt, &membership.UpdatedAt)
		if err != nil {
			return nil, err
		}
		memberships = append(memberships, membership)
	}
	return memberships, rows.Err()
}

func (store *SQLiteStore) UpdateMembership(ctx context.Context, membership *Membership) error {
	query := `UPDATE memberships SET role = ?, updated_at = ? WHERE id = ?`
	result, err := store.db.ExecContext(ctx, query, membership.Role, membership.UpdatedAt, membership.ID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrMemberNotFound
	}
	return nil
}

func (store *SQLiteStore) DeleteMembership(ctx context.Context, orgID, userID string) error {
	query := `DELETE FROM memberships WHERE org_id = ? AND user_id = ?`
	result, err := store.db.ExecContext(ctx, query, orgID, userID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrMemberNotFound
	}
	return nil
}

func (store *SQLiteStore) DeleteMembershipsByOrgID(ctx context.Context, orgID string) error {
	query := `DELETE FROM memberships WHERE org_id = ?`
	_, err := store.db.ExecContext(ctx, query, orgID)
	return err
}

func (store *SQLiteStore) Close() error {
	return store.db.Close()
}
