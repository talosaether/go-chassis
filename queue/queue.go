// Package queue provides background job processing for the chassis framework.
//
// Jobs are persisted to SQLite by default, ensuring durability across restarts.
// The module supports job types for routing, status tracking, and worker patterns.
//
// # Usage
//
// Register the module with chassis:
//
//	app := chassis.New(
//	    chassis.WithModules(
//	        queue.New(),
//	    ),
//	)
//
// Enqueue and process jobs:
//
//	// Enqueue a job
//	job, err := app.Queue().Enqueue(ctx, "send-email", map[string]string{
//	    "to":      "user@example.com",
//	    "subject": "Welcome!",
//	})
//
//	// Process jobs with a worker
//	go app.Queue().Worker(ctx, func(ctx context.Context, job *queue.Job) error {
//	    var payload map[string]string
//	    json.Unmarshal(job.Payload, &payload)
//	    // Process the job...
//	    return nil
//	})
//
// # Job Lifecycle
//
// Jobs progress through statuses: pending -> processing -> completed/failed.
// Failed jobs can be retried:
//
//	app.Queue().Retry(ctx, jobID)
//
// # Configuration
//
// Configure via config.yaml:
//
//	queue:
//	  db_path: ./data/queue.db
//
// Or programmatically:
//
//	queue.New(queue.WithDBPath("/custom/queue.db"))
package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/talosaether/chassis"
)

var (
	ErrJobNotFound = errors.New("job not found")
	ErrNoJobs      = errors.New("no jobs available")
)

// JobStatus represents the status of a job.
type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusCompleted  JobStatus = "completed"
	StatusFailed     JobStatus = "failed"
)

// Job represents a background job in the queue.
type Job struct {
	ID          string
	Type        string
	Payload     json.RawMessage
	Status      JobStatus
	Error       string
	CreatedAt   time.Time
	ProcessedAt *time.Time
}

// Module is the queue module implementation.
type Module struct {
	store  Store
	dbPath string
	app    *chassis.App
}

// Option is a function that configures the queue module.
type Option func(*Module)

// WithStore sets a custom store implementation.
func WithStore(store Store) Option {
	return func(mod *Module) {
		mod.store = store
	}
}

// WithDBPath sets the SQLite database path.
func WithDBPath(path string) Option {
	return func(mod *Module) {
		mod.dbPath = path
	}
}

// New creates a new queue module with the given options.
func New(opts ...Option) *Module {
	mod := &Module{
		dbPath: "./data/queue.db",
	}

	for _, opt := range opts {
		opt(mod)
	}

	return mod
}

// Name returns the module identifier.
func (mod *Module) Name() string {
	return "queue"
}

// Init initializes the queue module.
func (mod *Module) Init(ctx context.Context, app *chassis.App) error {
	mod.app = app

	// Read config if available
	if cfg := app.ConfigData(); cfg != nil {
		if dbPath := cfg.GetString("queue.db_path"); dbPath != "" {
			mod.dbPath = dbPath
		}
	}

	// Use default SQLite store if none provided
	if mod.store == nil {
		sqliteStore, err := NewSQLiteStore(mod.dbPath)
		if err != nil {
			return fmt.Errorf("failed to create queue store: %w", err)
		}
		mod.store = sqliteStore
		app.Logger().Info("queue module initialized", "db_path", mod.dbPath)
	} else {
		app.Logger().Info("queue module initialized with custom store")
	}

	return nil
}

// Shutdown cleans up the queue module.
func (mod *Module) Shutdown(ctx context.Context) error {
	if mod.store != nil {
		return mod.store.Close()
	}
	return nil
}

// Enqueue adds a new job to the queue.
func (mod *Module) Enqueue(ctx context.Context, jobType string, payload any) (any, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	job := &Job{
		ID:        uuid.New().String(),
		Type:      jobType,
		Payload:   payloadBytes,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}

	if err := mod.store.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to enqueue job: %w", err)
	}

	return job, nil
}

// Dequeue retrieves and claims the next pending job.
func (mod *Module) Dequeue(ctx context.Context) (any, error) {
	return mod.dequeue(ctx)
}

// dequeue is the internal implementation that returns *Job.
func (mod *Module) dequeue(ctx context.Context) (*Job, error) {
	return mod.store.Dequeue(ctx)
}

// DequeueByType retrieves and claims the next pending job of a specific type.
func (mod *Module) DequeueByType(ctx context.Context, jobType string) (any, error) {
	return mod.store.DequeueByType(ctx, jobType)
}

// Complete marks a job as completed.
func (mod *Module) Complete(ctx context.Context, jobID string) error {
	now := time.Now()
	return mod.store.UpdateStatus(ctx, jobID, StatusCompleted, "", &now)
}

// Fail marks a job as failed with an error message.
func (mod *Module) Fail(ctx context.Context, jobID string, err error) error {
	now := time.Now()
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	return mod.store.UpdateStatus(ctx, jobID, StatusFailed, errMsg, &now)
}

// GetByID retrieves a job by its ID.
func (mod *Module) GetByID(ctx context.Context, jobID string) (any, error) {
	return mod.store.GetByID(ctx, jobID)
}

// GetAll retrieves all jobs regardless of status.
func (mod *Module) GetAll(ctx context.Context) (any, error) {
	return mod.store.GetAll(ctx)
}

// GetPending retrieves all pending jobs.
func (mod *Module) GetPending(ctx context.Context) (any, error) {
	return mod.store.GetByStatus(ctx, StatusPending)
}

// GetCompleted retrieves all completed jobs.
func (mod *Module) GetCompleted(ctx context.Context) (any, error) {
	return mod.store.GetByStatus(ctx, StatusCompleted)
}

// GetFailed retrieves all failed jobs.
func (mod *Module) GetFailed(ctx context.Context) (any, error) {
	return mod.store.GetByStatus(ctx, StatusFailed)
}

// Retry moves a failed job back to pending status.
func (mod *Module) Retry(ctx context.Context, jobID string) error {
	return mod.store.UpdateStatus(ctx, jobID, StatusPending, "", nil)
}

// Handler is a function that processes a job.
type Handler func(ctx context.Context, job *Job) error

// Worker processes jobs in a loop.
// It runs until the context is cancelled.
func (mod *Module) Worker(ctx context.Context, handler Handler) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			job, err := mod.dequeue(ctx)
			if err != nil {
				if errors.Is(err, ErrNoJobs) || errors.Is(err, ErrJobNotFound) {
					// No jobs available, wait before checking again
					time.Sleep(time.Second)
					continue
				}
				mod.app.Logger().Error("failed to dequeue job", "error", err)
				time.Sleep(time.Second)
				continue
			}

			if err := handler(ctx, job); err != nil {
				if failErr := mod.Fail(ctx, job.ID, err); failErr != nil {
					mod.app.Logger().Error("failed to mark job as failed", "job_id", job.ID, "error", failErr)
				}
				mod.app.Logger().Error("job failed", "job_id", job.ID, "type", job.Type, "error", err)
			} else {
				if completeErr := mod.Complete(ctx, job.ID); completeErr != nil {
					mod.app.Logger().Error("failed to mark job as complete", "job_id", job.ID, "error", completeErr)
				}
				mod.app.Logger().Info("job completed", "job_id", job.ID, "type", job.Type)
			}
		}
	}
}
