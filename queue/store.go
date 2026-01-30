package queue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// Store defines the interface for queue persistence.
type Store interface {
	Create(ctx context.Context, job *Job) error
	GetByID(ctx context.Context, id string) (*Job, error)
	GetByStatus(ctx context.Context, status JobStatus) ([]*Job, error)
	Dequeue(ctx context.Context) (*Job, error)
	DequeueByType(ctx context.Context, jobType string) (*Job, error)
	UpdateStatus(ctx context.Context, id string, status JobStatus, errMsg string, processedAt *time.Time) error
	Close() error
}

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite-backed queue store.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("failed to create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := initQueueSchema(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

func initQueueSchema(db *sql.DB) error {
	schema := `
		CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			payload BLOB,
			status TEXT NOT NULL DEFAULT 'pending',
			error TEXT,
			created_at DATETIME NOT NULL,
			processed_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
		CREATE INDEX IF NOT EXISTS idx_jobs_type_status ON jobs(type, status);
	`
	_, err := db.Exec(schema)
	return err
}

func (store *SQLiteStore) Create(ctx context.Context, job *Job) error {
	query := `INSERT INTO jobs (id, type, payload, status, created_at) VALUES (?, ?, ?, ?, ?)`
	_, err := store.db.ExecContext(ctx, query, job.ID, job.Type, job.Payload, job.Status, job.CreatedAt)
	return err
}

func (store *SQLiteStore) GetByID(ctx context.Context, id string) (*Job, error) {
	query := `SELECT id, type, payload, status, error, created_at, processed_at FROM jobs WHERE id = ?`
	row := store.db.QueryRowContext(ctx, query, id)
	job, err := scanJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrJobNotFound
		}
		return nil, err
	}
	return job, nil
}

func (store *SQLiteStore) GetByStatus(ctx context.Context, status JobStatus) ([]*Job, error) {
	query := `SELECT id, type, payload, status, error, created_at, processed_at FROM jobs WHERE status = ? ORDER BY created_at ASC`
	rows, err := store.db.QueryContext(ctx, query, status)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var jobs []*Job
	for rows.Next() {
		job, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

func (store *SQLiteStore) Dequeue(ctx context.Context) (*Job, error) {
	// Use a transaction to atomically claim a job
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	// Select the oldest pending job
	selectQuery := `SELECT id, type, payload, status, error, created_at, processed_at FROM jobs WHERE status = ? ORDER BY created_at ASC LIMIT 1`
	row := tx.QueryRowContext(ctx, selectQuery, StatusPending)

	job, err := scanJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoJobs
		}
		return nil, err
	}

	// Update status to processing
	updateQuery := `UPDATE jobs SET status = ? WHERE id = ?`
	_, err = tx.ExecContext(ctx, updateQuery, StatusProcessing, job.ID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	job.Status = StatusProcessing
	return job, nil
}

func (store *SQLiteStore) DequeueByType(ctx context.Context, jobType string) (*Job, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	selectQuery := `SELECT id, type, payload, status, error, created_at, processed_at FROM jobs WHERE status = ? AND type = ? ORDER BY created_at ASC LIMIT 1`
	row := tx.QueryRowContext(ctx, selectQuery, StatusPending, jobType)

	job, err := scanJob(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNoJobs
		}
		return nil, err
	}

	updateQuery := `UPDATE jobs SET status = ? WHERE id = ?`
	_, err = tx.ExecContext(ctx, updateQuery, StatusProcessing, job.ID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	job.Status = StatusProcessing
	return job, nil
}

func (store *SQLiteStore) UpdateStatus(ctx context.Context, id string, status JobStatus, errMsg string, processedAt *time.Time) error {
	query := `UPDATE jobs SET status = ?, error = ?, processed_at = ? WHERE id = ?`
	result, err := store.db.ExecContext(ctx, query, status, errMsg, processedAt, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrJobNotFound
	}
	return nil
}

func (store *SQLiteStore) Close() error {
	return store.db.Close()
}

func scanJob(row *sql.Row) (*Job, error) {
	var job Job
	var payload []byte
	var errMsg sql.NullString
	var processedAt sql.NullTime

	err := row.Scan(&job.ID, &job.Type, &payload, &job.Status, &errMsg, &job.CreatedAt, &processedAt)
	if err != nil {
		// Return sql.ErrNoRows directly so callers can map it appropriately
		return nil, err
	}

	if payload != nil {
		job.Payload = payload
	}
	if errMsg.Valid {
		job.Error = errMsg.String
	}
	if processedAt.Valid {
		job.ProcessedAt = &processedAt.Time
	}

	return &job, nil
}

func scanJobRow(rows *sql.Rows) (*Job, error) {
	var job Job
	var payload []byte
	var errMsg sql.NullString
	var processedAt sql.NullTime

	err := rows.Scan(&job.ID, &job.Type, &payload, &job.Status, &errMsg, &job.CreatedAt, &processedAt)
	if err != nil {
		return nil, err
	}

	if payload != nil {
		job.Payload = payload
	}
	if errMsg.Valid {
		job.Error = errMsg.String
	}
	if processedAt.Valid {
		job.ProcessedAt = &processedAt.Time
	}

	return &job, nil
}
