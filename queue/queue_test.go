package queue

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func setupTestStore(t *testing.T) (*SQLiteStore, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-queue.db")

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}

	cleanup := func() {
		_ = store.Close()
		_ = os.Remove(dbPath)
	}

	return store, cleanup
}

func TestSQLiteStore_CreateAndGetByID(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	job := &Job{
		ID:      "test-job-id",
		Type:    "email",
		Payload: json.RawMessage(`{"to":"test@example.com"}`),
		Status:  StatusPending,
	}

	err := store.Create(ctx, job)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := store.GetByID(ctx, "test-job-id")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if got.ID != job.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, job.ID)
	}
	if got.Type != job.Type {
		t.Errorf("Type mismatch: got %q, want %q", got.Type, job.Type)
	}
	if got.Status != StatusPending {
		t.Errorf("Status should be pending, got %q", got.Status)
	}
}

func TestSQLiteStore_GetByIDNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetByID(ctx, "nonexistent")
	if !errors.Is(err, ErrJobNotFound) {
		t.Errorf("expected ErrJobNotFound, got: %v", err)
	}
}

func TestSQLiteStore_Dequeue(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create jobs
	job1 := &Job{ID: "job1", Type: "task", Status: StatusPending}
	job2 := &Job{ID: "job2", Type: "task", Status: StatusPending}
	store.Create(ctx, job1)
	store.Create(ctx, job2)

	// Dequeue first job
	got, err := store.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	if got.ID != "job1" {
		t.Errorf("expected job1, got %q", got.ID)
	}
	if got.Status != StatusProcessing {
		t.Errorf("status should be processing, got %q", got.Status)
	}

	// Dequeue second job
	got2, err := store.Dequeue(ctx)
	if err != nil {
		t.Fatalf("second Dequeue failed: %v", err)
	}
	if got2.ID != "job2" {
		t.Errorf("expected job2, got %q", got2.ID)
	}
}

func TestSQLiteStore_DequeueEmpty(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.Dequeue(ctx)
	if !errors.Is(err, ErrNoJobs) {
		t.Errorf("expected ErrNoJobs, got: %v", err)
	}
}

func TestSQLiteStore_DequeueByType(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	store.Create(ctx, &Job{ID: "email1", Type: "email", Status: StatusPending})
	store.Create(ctx, &Job{ID: "sms1", Type: "sms", Status: StatusPending})
	store.Create(ctx, &Job{ID: "email2", Type: "email", Status: StatusPending})

	// Dequeue only email jobs
	got, err := store.DequeueByType(ctx, "email")
	if err != nil {
		t.Fatalf("DequeueByType failed: %v", err)
	}

	if got.Type != "email" {
		t.Errorf("expected email type, got %q", got.Type)
	}
}

func TestSQLiteStore_GetByStatus(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	store.Create(ctx, &Job{ID: "job1", Type: "task", Status: StatusPending})
	store.Create(ctx, &Job{ID: "job2", Type: "task", Status: StatusPending})
	store.Create(ctx, &Job{ID: "job3", Type: "task", Status: StatusCompleted})

	pending, err := store.GetByStatus(ctx, StatusPending)
	if err != nil {
		t.Fatalf("GetByStatus failed: %v", err)
	}

	if len(pending) != 2 {
		t.Errorf("expected 2 pending jobs, got %d", len(pending))
	}
}

func TestSQLiteStore_GetAll(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	store.Create(ctx, &Job{ID: "job1", Type: "task", Status: StatusPending})
	store.Create(ctx, &Job{ID: "job2", Type: "task", Status: StatusProcessing})
	store.Create(ctx, &Job{ID: "job3", Type: "task", Status: StatusCompleted})
	store.Create(ctx, &Job{ID: "job4", Type: "task", Status: StatusFailed})

	all, err := store.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if len(all) != 4 {
		t.Errorf("expected 4 jobs, got %d", len(all))
	}
}

func TestSQLiteStore_GetAllEmpty(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	all, err := store.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	if all == nil {
		t.Error("GetAll should return empty slice, not nil")
	}
	if len(all) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(all))
	}
}

func TestSQLiteStore_UpdateStatus(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	job := &Job{ID: "job1", Type: "task", Status: StatusPending}
	store.Create(ctx, job)

	err := store.UpdateStatus(ctx, "job1", StatusCompleted, "", nil)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	got, _ := store.GetByID(ctx, "job1")
	if got.Status != StatusCompleted {
		t.Errorf("status should be completed, got %q", got.Status)
	}
}

func TestSQLiteStore_UpdateStatusWithError(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	job := &Job{ID: "job1", Type: "task", Status: StatusProcessing}
	store.Create(ctx, job)

	err := store.UpdateStatus(ctx, "job1", StatusFailed, "connection timeout", nil)
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	got, _ := store.GetByID(ctx, "job1")
	if got.Status != StatusFailed {
		t.Errorf("status should be failed, got %q", got.Status)
	}
	if got.Error != "connection timeout" {
		t.Errorf("error should be 'connection timeout', got %q", got.Error)
	}
}

func TestSQLiteStore_UpdateStatusNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	err := store.UpdateStatus(ctx, "nonexistent", StatusCompleted, "", nil)
	if !errors.Is(err, ErrJobNotFound) {
		t.Errorf("expected ErrJobNotFound, got: %v", err)
	}
}

// Module tests

func TestModule_Name(t *testing.T) {
	mod := New()
	if mod.Name() != "queue" {
		t.Errorf("Name() should return 'queue', got %q", mod.Name())
	}
}

func TestModuleNew_DefaultOptions(t *testing.T) {
	mod := New()
	if mod.dbPath != "./data/queue.db" {
		t.Errorf("default dbPath should be './data/queue.db', got %q", mod.dbPath)
	}
}

func TestModuleNew_WithDBPath(t *testing.T) {
	mod := New(WithDBPath("/custom/queue.db"))
	if mod.dbPath != "/custom/queue.db" {
		t.Errorf("dbPath should be '/custom/queue.db', got %q", mod.dbPath)
	}
}

func TestModuleNew_WithStore(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	if mod.store != store {
		t.Error("custom store should be set")
	}
}

func TestModule_EnqueueAndDequeue(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	payload := map[string]string{"key": "value"}

	jobResult, err := mod.Enqueue(ctx, "test-type", payload)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	job := jobResult.(*Job)
	if job.Type != "test-type" {
		t.Errorf("type should be 'test-type', got %q", job.Type)
	}

	// Dequeue
	dequeuedResult, err := mod.Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	dequeued := dequeuedResult.(*Job)
	if dequeued.ID != job.ID {
		t.Errorf("dequeued job ID mismatch: got %q, want %q", dequeued.ID, job.ID)
	}
}

func TestModule_Complete(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	jobResult, err := mod.Enqueue(ctx, "task", nil)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	job := jobResult.(*Job)

	// Claim it
	if _, err := mod.Dequeue(ctx); err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	// Complete it
	err = mod.Complete(ctx, job.ID)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	gotResult, _ := mod.GetByID(ctx, job.ID)
	got := gotResult.(*Job)
	if got.Status != StatusCompleted {
		t.Errorf("status should be completed, got %q", got.Status)
	}
	if got.ProcessedAt == nil {
		t.Error("ProcessedAt should be set")
	}
}

func TestModule_Fail(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	jobResult, err := mod.Enqueue(ctx, "task", nil)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	job := jobResult.(*Job)

	if _, err := mod.Dequeue(ctx); err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	testErr := errors.New("task failed")
	err = mod.Fail(ctx, job.ID, testErr)
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}

	gotResult, _ := mod.GetByID(ctx, job.ID)
	got := gotResult.(*Job)
	if got.Status != StatusFailed {
		t.Errorf("status should be failed, got %q", got.Status)
	}
	if got.Error != "task failed" {
		t.Errorf("error should be 'task failed', got %q", got.Error)
	}
}

func TestModule_Retry(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	jobResult, err := mod.Enqueue(ctx, "task", nil)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	job := jobResult.(*Job)

	if _, err := mod.Dequeue(ctx); err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	mod.Fail(ctx, job.ID, errors.New("first attempt failed"))

	// Retry
	err = mod.Retry(ctx, job.ID)
	if err != nil {
		t.Fatalf("Retry failed: %v", err)
	}

	gotResult, _ := mod.GetByID(ctx, job.ID)
	got := gotResult.(*Job)
	if got.Status != StatusPending {
		t.Errorf("status should be pending after retry, got %q", got.Status)
	}
}

func TestModule_GetAll(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	// Create jobs with different statuses
	job1, _ := mod.Enqueue(ctx, "task1", nil)
	mod.Enqueue(ctx, "task2", nil)

	// Complete one job
	mod.Dequeue(ctx)
	mod.Complete(ctx, job1.(*Job).ID)

	allResult, err := mod.GetAll(ctx)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	all := allResult.([]*Job)
	if len(all) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(all))
	}
}

func TestModule_GetPending(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	mod.Enqueue(ctx, "task1", nil)
	mod.Enqueue(ctx, "task2", nil)

	pendingResult, err := mod.GetPending(ctx)
	if err != nil {
		t.Fatalf("GetPending failed: %v", err)
	}

	pending := pendingResult.([]*Job)
	if len(pending) != 2 {
		t.Errorf("expected 2 pending jobs, got %d", len(pending))
	}
}

func TestModule_GetCompleted(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	// Create and complete a job
	job1, _ := mod.Enqueue(ctx, "task1", nil)
	mod.Enqueue(ctx, "task2", nil) // stays pending

	mod.Dequeue(ctx)
	mod.Complete(ctx, job1.(*Job).ID)

	completedResult, err := mod.GetCompleted(ctx)
	if err != nil {
		t.Fatalf("GetCompleted failed: %v", err)
	}

	completed := completedResult.([]*Job)
	if len(completed) != 1 {
		t.Errorf("expected 1 completed job, got %d", len(completed))
	}
	if completed[0].Status != StatusCompleted {
		t.Errorf("expected completed status, got %q", completed[0].Status)
	}
}

func TestModule_GetFailed(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	jobResult, err := mod.Enqueue(ctx, "task", nil)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	job := jobResult.(*Job)

	if _, err := mod.Dequeue(ctx); err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}
	mod.Fail(ctx, job.ID, errors.New("failed"))

	failedResult, err := mod.GetFailed(ctx)
	if err != nil {
		t.Fatalf("GetFailed failed: %v", err)
	}

	failed := failedResult.([]*Job)
	if len(failed) != 1 {
		t.Errorf("expected 1 failed job, got %d", len(failed))
	}
}

func TestJobStatus_Constants(t *testing.T) {
	// Verify status constants
	if StatusPending != "pending" {
		t.Errorf("StatusPending should be 'pending', got %q", StatusPending)
	}
	if StatusProcessing != "processing" {
		t.Errorf("StatusProcessing should be 'processing', got %q", StatusProcessing)
	}
	if StatusCompleted != "completed" {
		t.Errorf("StatusCompleted should be 'completed', got %q", StatusCompleted)
	}
	if StatusFailed != "failed" {
		t.Errorf("StatusFailed should be 'failed', got %q", StatusFailed)
	}
}
