package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func TestSQLiteStore_GetAllPaginated(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create 10 jobs
	for i := 0; i < 10; i++ {
		store.Create(ctx, &Job{ID: fmt.Sprintf("job%d", i), Type: "task", Status: StatusPending})
	}

	// Get first page (5 items)
	page1, err := store.GetAllPaginated(ctx, 0, 5)
	if err != nil {
		t.Fatalf("GetAllPaginated failed: %v", err)
	}
	if len(page1) != 5 {
		t.Errorf("expected 5 jobs on page 1, got %d", len(page1))
	}

	// Get second page (5 items)
	page2, err := store.GetAllPaginated(ctx, 5, 5)
	if err != nil {
		t.Fatalf("GetAllPaginated page 2 failed: %v", err)
	}
	if len(page2) != 5 {
		t.Errorf("expected 5 jobs on page 2, got %d", len(page2))
	}

	// Ensure pages have different jobs
	if page1[0].ID == page2[0].ID {
		t.Error("page 1 and page 2 should have different jobs")
	}

	// Get third page (should be empty)
	page3, err := store.GetAllPaginated(ctx, 10, 5)
	if err != nil {
		t.Fatalf("GetAllPaginated page 3 failed: %v", err)
	}
	if len(page3) != 0 {
		t.Errorf("expected 0 jobs on page 3, got %d", len(page3))
	}
}

func TestSQLiteStore_GetByStatusPaginated(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create 6 pending and 4 completed jobs
	for i := 0; i < 6; i++ {
		store.Create(ctx, &Job{ID: fmt.Sprintf("pending%d", i), Type: "task", Status: StatusPending})
	}
	for i := 0; i < 4; i++ {
		store.Create(ctx, &Job{ID: fmt.Sprintf("completed%d", i), Type: "task", Status: StatusCompleted})
	}

	// Get first page of pending (3 items)
	pending1, err := store.GetByStatusPaginated(ctx, StatusPending, 0, 3)
	if err != nil {
		t.Fatalf("GetByStatusPaginated failed: %v", err)
	}
	if len(pending1) != 3 {
		t.Errorf("expected 3 pending jobs, got %d", len(pending1))
	}

	// Verify all are pending
	for _, job := range pending1 {
		if job.Status != StatusPending {
			t.Errorf("expected pending status, got %s", job.Status)
		}
	}

	// Get second page of pending
	pending2, err := store.GetByStatusPaginated(ctx, StatusPending, 3, 3)
	if err != nil {
		t.Fatalf("GetByStatusPaginated page 2 failed: %v", err)
	}
	if len(pending2) != 3 {
		t.Errorf("expected 3 pending jobs on page 2, got %d", len(pending2))
	}

	// Get completed jobs
	completed, err := store.GetByStatusPaginated(ctx, StatusCompleted, 0, 10)
	if err != nil {
		t.Fatalf("GetByStatusPaginated completed failed: %v", err)
	}
	if len(completed) != 4 {
		t.Errorf("expected 4 completed jobs, got %d", len(completed))
	}
}

func TestSQLiteStore_CountAll(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Empty count
	count, err := store.CountAll(ctx)
	if err != nil {
		t.Fatalf("CountAll failed: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}

	// Add jobs
	store.Create(ctx, &Job{ID: "job1", Type: "task", Status: StatusPending})
	store.Create(ctx, &Job{ID: "job2", Type: "task", Status: StatusCompleted})
	store.Create(ctx, &Job{ID: "job3", Type: "task", Status: StatusPending})

	count, err = store.CountAll(ctx)
	if err != nil {
		t.Fatalf("CountAll failed: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestSQLiteStore_CountByStatus(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	store.Create(ctx, &Job{ID: "job1", Type: "task", Status: StatusPending})
	store.Create(ctx, &Job{ID: "job2", Type: "task", Status: StatusPending})
	store.Create(ctx, &Job{ID: "job3", Type: "task", Status: StatusCompleted})

	pendingCount, err := store.CountByStatus(ctx, StatusPending)
	if err != nil {
		t.Fatalf("CountByStatus pending failed: %v", err)
	}
	if pendingCount != 2 {
		t.Errorf("expected 2 pending, got %d", pendingCount)
	}

	completedCount, err := store.CountByStatus(ctx, StatusCompleted)
	if err != nil {
		t.Fatalf("CountByStatus completed failed: %v", err)
	}
	if completedCount != 1 {
		t.Errorf("expected 1 completed, got %d", completedCount)
	}

	failedCount, err := store.CountByStatus(ctx, StatusFailed)
	if err != nil {
		t.Fatalf("CountByStatus failed failed: %v", err)
	}
	if failedCount != 0 {
		t.Errorf("expected 0 failed, got %d", failedCount)
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

func TestModule_GetByID(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	// Enqueue a job
	jobResult, err := mod.Enqueue(ctx, "test-type", map[string]string{"data": "value"})
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}
	job := jobResult.(*Job)

	// Get by ID
	gotResult, err := mod.GetByID(ctx, job.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	got := gotResult.(*Job)
	if got.ID != job.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, job.ID)
	}
	if got.Type != "test-type" {
		t.Errorf("Type mismatch: got %q, want %q", got.Type, "test-type")
	}
	if got.Status != StatusPending {
		t.Errorf("Status mismatch: got %q, want %q", got.Status, StatusPending)
	}
}

func TestModule_GetByIDNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	_, err := mod.GetByID(ctx, "nonexistent-id")
	if err == nil {
		t.Error("GetByID should return error for nonexistent job")
	}
	if !errors.Is(err, ErrJobNotFound) {
		t.Errorf("expected ErrJobNotFound, got: %v", err)
	}
}

func TestModule_GetAllPaginated(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	// Create 15 jobs
	for i := 0; i < 15; i++ {
		mod.Enqueue(ctx, fmt.Sprintf("type%d", i), nil)
	}

	// Get first page
	result, err := mod.GetAllPaginated(ctx, 1, 5)
	if err != nil {
		t.Fatalf("GetAllPaginated failed: %v", err)
	}

	if len(result.Jobs) != 5 {
		t.Errorf("expected 5 jobs, got %d", len(result.Jobs))
	}
	if result.Page != 1 {
		t.Errorf("expected page 1, got %d", result.Page)
	}
	if result.Limit != 5 {
		t.Errorf("expected limit 5, got %d", result.Limit)
	}
	if result.Total != 15 {
		t.Errorf("expected total 15, got %d", result.Total)
	}
	if result.TotalPages != 3 {
		t.Errorf("expected 3 total pages, got %d", result.TotalPages)
	}

	// Get second page
	result2, err := mod.GetAllPaginated(ctx, 2, 5)
	if err != nil {
		t.Fatalf("GetAllPaginated page 2 failed: %v", err)
	}
	if len(result2.Jobs) != 5 {
		t.Errorf("expected 5 jobs on page 2, got %d", len(result2.Jobs))
	}
	if result2.Page != 2 {
		t.Errorf("expected page 2, got %d", result2.Page)
	}

	// Get third page (partial)
	result3, err := mod.GetAllPaginated(ctx, 3, 5)
	if err != nil {
		t.Fatalf("GetAllPaginated page 3 failed: %v", err)
	}
	if len(result3.Jobs) != 5 {
		t.Errorf("expected 5 jobs on page 3, got %d", len(result3.Jobs))
	}

	// Get fourth page (empty)
	result4, err := mod.GetAllPaginated(ctx, 4, 5)
	if err != nil {
		t.Fatalf("GetAllPaginated page 4 failed: %v", err)
	}
	if len(result4.Jobs) != 0 {
		t.Errorf("expected 0 jobs on page 4, got %d", len(result4.Jobs))
	}
}

func TestModule_GetAllPaginated_DefaultValues(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	mod.Enqueue(ctx, "task", nil)

	// Test with invalid page (should default to 1)
	result, err := mod.GetAllPaginated(ctx, 0, 10)
	if err != nil {
		t.Fatalf("GetAllPaginated failed: %v", err)
	}
	if result.Page != 1 {
		t.Errorf("expected page 1 for invalid input, got %d", result.Page)
	}

	// Test with invalid limit (should default to 20)
	result2, err := mod.GetAllPaginated(ctx, 1, 0)
	if err != nil {
		t.Fatalf("GetAllPaginated failed: %v", err)
	}
	if result2.Limit != 20 {
		t.Errorf("expected limit 20 for invalid input, got %d", result2.Limit)
	}
}

func TestModule_GetByStatusPaginated(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	// Create 8 pending jobs
	for i := 0; i < 8; i++ {
		mod.Enqueue(ctx, "pending-task", nil)
	}

	// Complete 3 of them
	for i := 0; i < 3; i++ {
		job, _ := mod.Dequeue(ctx)
		mod.Complete(ctx, job.(*Job).ID)
	}

	// Get pending jobs paginated
	pendingResult, err := mod.GetByStatusPaginated(ctx, StatusPending, 1, 3)
	if err != nil {
		t.Fatalf("GetByStatusPaginated failed: %v", err)
	}
	if len(pendingResult.Jobs) != 3 {
		t.Errorf("expected 3 pending jobs, got %d", len(pendingResult.Jobs))
	}
	if pendingResult.Total != 5 {
		t.Errorf("expected 5 total pending, got %d", pendingResult.Total)
	}
	if pendingResult.TotalPages != 2 {
		t.Errorf("expected 2 total pages, got %d", pendingResult.TotalPages)
	}

	// Get completed jobs paginated
	completedResult, err := mod.GetByStatusPaginated(ctx, StatusCompleted, 1, 10)
	if err != nil {
		t.Fatalf("GetByStatusPaginated completed failed: %v", err)
	}
	if len(completedResult.Jobs) != 3 {
		t.Errorf("expected 3 completed jobs, got %d", len(completedResult.Jobs))
	}
	if completedResult.Total != 3 {
		t.Errorf("expected 3 total completed, got %d", completedResult.Total)
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
