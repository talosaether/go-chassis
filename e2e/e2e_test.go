package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/talosaether/chassis"
	"github.com/talosaether/chassis/auth"
	"github.com/talosaether/chassis/cache"
	"github.com/talosaether/chassis/email"
	"github.com/talosaether/chassis/events"
	"github.com/talosaether/chassis/orgs"
	"github.com/talosaether/chassis/permissions"
	"github.com/talosaether/chassis/queue"
	"github.com/talosaether/chassis/storage"
	"github.com/talosaether/chassis/users"
)

// setupTestApp creates a fully configured App with all modules using temp directories.
func setupTestApp(t *testing.T) (*chassis.App, func()) {
	t.Helper()

	tmpDir := t.TempDir()

	// Create email provider that captures sent emails
	var sentEmails []struct {
		To, Subject, Body string
	}
	var emailMu sync.Mutex
	emailProvider := email.NewLogProvider(func(to, subject, body string) {
		emailMu.Lock()
		defer emailMu.Unlock()
		sentEmails = append(sentEmails, struct{ To, Subject, Body string }{to, subject, body})
	})

	app := chassis.New(
		chassis.WithModules(
			storage.New(storage.WithBasePath(filepath.Join(tmpDir, "storage"))),
			users.New(users.WithDBPath(filepath.Join(tmpDir, "users.db"))),
			auth.New(auth.WithDBPath(filepath.Join(tmpDir, "sessions.db"))),
			orgs.New(orgs.WithDBPath(filepath.Join(tmpDir, "orgs.db"))),
			permissions.New(),
			cache.New(),
			queue.New(queue.WithDBPath(filepath.Join(tmpDir, "queue.db"))),
			email.New(email.WithProvider(emailProvider)),
			events.New(),
		),
	)

	cleanup := func() {
		ctx := context.Background()
		_ = app.Shutdown(ctx)
		_ = os.RemoveAll(tmpDir)
	}

	return app, cleanup
}

// TestAppLifecycle tests basic app creation and shutdown.
func TestAppLifecycle(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	// Verify all modules are accessible (no panic)
	_ = app.Storage()
	_ = app.Users()
	_ = app.Auth()
	_ = app.Orgs()
	_ = app.Permissions()
	_ = app.Cache()
	_ = app.Queue()
	_ = app.Email()
	_ = app.Events()
}

// TestUserRegistrationAndAuthentication tests the full user flow.
func TestUserRegistrationAndAuthentication(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	userResult, err := app.Users().Create(ctx, "test@example.com", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	user := userResult.(*users.User)
	if user.GetEmail() != "test@example.com" {
		t.Errorf("email mismatch: got %q, want %q", user.GetEmail(), "test@example.com")
	}
	if user.GetID() == "" {
		t.Error("user ID should not be empty")
	}

	// Authenticate the user
	authResult, err := app.Users().Authenticate(ctx, "test@example.com", "password123")
	if err != nil {
		t.Fatalf("authentication failed: %v", err)
	}

	authUser := authResult.(*users.User)
	if authUser.GetID() != user.GetID() {
		t.Errorf("authenticated user ID mismatch: got %q, want %q", authUser.GetID(), user.GetID())
	}

	// Authentication with wrong password should fail
	_, err = app.Users().Authenticate(ctx, "test@example.com", "wrongpassword")
	if err == nil {
		t.Error("authentication with wrong password should fail")
	}

	// Get user by ID
	foundResult, err := app.Users().GetByID(ctx, user.GetID())
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	foundUser := foundResult.(*users.User)
	if foundUser.GetEmail() != "test@example.com" {
		t.Errorf("found user email mismatch: got %q", foundUser.GetEmail())
	}

	// Get user by email
	foundByEmailResult, err := app.Users().GetByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("GetByEmail failed: %v", err)
	}
	foundByEmail := foundByEmailResult.(*users.User)
	if foundByEmail.GetID() != user.GetID() {
		t.Errorf("found by email user ID mismatch")
	}
}

// TestOrganizationAndMembershipFlow tests org creation, membership, and permissions.
func TestOrganizationAndMembershipFlow(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	ctx := context.Background()

	// Create users
	ownerResult, _ := app.Users().Create(ctx, "owner@example.com", "password123")
	owner := ownerResult.(*users.User)

	adminResult, _ := app.Users().Create(ctx, "admin@example.com", "password123")
	admin := adminResult.(*users.User)

	memberResult, _ := app.Users().Create(ctx, "member@example.com", "password123")
	member := memberResult.(*users.User)

	nonMemberResult, _ := app.Users().Create(ctx, "nonmember@example.com", "password123")
	nonMember := nonMemberResult.(*users.User)

	// Create an organization
	orgResult, err := app.Orgs().Create(ctx, orgs.CreateInput{Name: "Acme Corp"})
	if err != nil {
		t.Fatalf("failed to create org: %v", err)
	}
	org := orgResult.(*orgs.Org)

	if org.Name != "Acme Corp" {
		t.Errorf("org name mismatch: got %q", org.Name)
	}

	// Add members with different roles
	app.Orgs().AddMember(ctx, org.ID(), owner.GetID(), "owner")
	app.Orgs().AddMember(ctx, org.ID(), admin.GetID(), "admin")
	app.Orgs().AddMember(ctx, org.ID(), member.GetID(), "member")

	// Test permission checks
	tests := []struct {
		name       string
		userID     string
		permission string
		expected   bool
	}{
		{"owner can read", owner.GetID(), "org:read", true},
		{"owner can delete", owner.GetID(), "org:delete", true},
		{"owner can manage roles", owner.GetID(), "org:manage_roles", true},
		{"admin can read", admin.GetID(), "org:read", true},
		{"admin can delete", admin.GetID(), "org:delete", true},
		{"admin cannot manage roles", admin.GetID(), "org:manage_roles", false},
		{"member can read", member.GetID(), "org:read", true},
		{"member cannot delete", member.GetID(), "org:delete", false},
		{"member cannot manage roles", member.GetID(), "org:manage_roles", false},
		{"non-member cannot read", nonMember.GetID(), "org:read", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			can := app.Permissions().Can(ctx, tt.userID, tt.permission, org.ID())
			if can != tt.expected {
				t.Errorf("Can(%s, %s, %s) = %v, want %v",
					tt.userID, tt.permission, org.ID(), can, tt.expected)
			}
		})
	}

	// Test GetUserRole
	ownerRole := app.Orgs().GetUserRole(ctx, org.ID(), owner.GetID())
	if ownerRole != "owner" {
		t.Errorf("owner role mismatch: got %q", ownerRole)
	}

	// Test GetMembers
	membersResult, err := app.Orgs().GetMembers(ctx, org.ID())
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	members := membersResult.([]*orgs.Membership)
	if len(members) != 3 {
		t.Errorf("expected 3 members, got %d", len(members))
	}

	// Test RemoveMember
	err = app.Orgs().RemoveMember(ctx, org.ID(), member.GetID())
	if err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}

	// Member should no longer have access
	canRead := app.Permissions().Can(ctx, member.GetID(), "org:read", org.ID())
	if canRead {
		t.Error("removed member should not be able to read org")
	}
}

// TestStorageOperations tests file storage across the app.
func TestStorageOperations(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	ctx := context.Background()

	// Put a file
	content := []byte("Hello, World!")
	app.Storage().Put(ctx, "docs/hello.txt", content)

	// Get the file
	data, err := app.Storage().Get(ctx, "docs/hello.txt")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(data) != "Hello, World!" {
		t.Errorf("content mismatch: got %q", string(data))
	}

	// Put another file
	app.Storage().Put(ctx, "docs/goodbye.txt", []byte("Goodbye!"))

	// List files
	files, err := app.Storage().List(ctx, "docs/")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}

	// Delete a file
	err = app.Storage().Delete(ctx, "docs/hello.txt")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// File should not exist
	_, err = app.Storage().Get(ctx, "docs/hello.txt")
	if !os.IsNotExist(err) {
		t.Errorf("expected file not found error, got: %v", err)
	}
}

// TestCacheOperations tests caching functionality.
func TestCacheOperations(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	ctx := context.Background()

	// Set a value
	app.Cache().Set(ctx, "user:123", []byte(`{"name":"John"}`))

	// Get the value
	data, found := app.Cache().Get(ctx, "user:123")
	if !found {
		t.Fatal("cached value not found")
	}
	if string(data) != `{"name":"John"}` {
		t.Errorf("value mismatch: got %q", string(data))
	}

	// Get non-existent key
	_, found = app.Cache().Get(ctx, "nonexistent")
	if found {
		t.Error("should not find non-existent key")
	}

	// Delete the value
	app.Cache().Delete(ctx, "user:123")

	_, found = app.Cache().Get(ctx, "user:123")
	if found {
		t.Error("value should be deleted")
	}

	// Test Clear
	app.Cache().Set(ctx, "key1", []byte("value1"))
	app.Cache().Set(ctx, "key2", []byte("value2"))

	app.Cache().Clear(ctx)

	_, found = app.Cache().Get(ctx, "key1")
	if found {
		t.Error("key1 should be cleared")
	}
}

// TestQueueJobProcessing tests job queue operations.
func TestQueueJobProcessing(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	ctx := context.Background()

	// Enqueue a job
	jobResult, err := app.Queue().Enqueue(ctx, "send_email", map[string]string{
		"to":      "user@example.com",
		"subject": "Welcome",
	})
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	job := jobResult.(*queue.Job)
	if job.Type != "send_email" {
		t.Errorf("job type mismatch: got %q", job.Type)
	}
	if job.Status != queue.StatusPending {
		t.Errorf("job status should be pending: got %q", job.Status)
	}

	// Dequeue the job
	dequeuedResult, err := app.Queue().Dequeue(ctx)
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	dequeued := dequeuedResult.(*queue.Job)
	if dequeued.ID != job.ID {
		t.Errorf("dequeued job ID mismatch: got %q, want %q", dequeued.ID, job.ID)
	}
	if dequeued.Status != queue.StatusProcessing {
		t.Errorf("dequeued job should be processing: got %q", dequeued.Status)
	}

	// Complete the job
	err = app.Queue().Complete(ctx, dequeued.ID)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	// Enqueue and fail a job
	failJobResult, err := app.Queue().Enqueue(ctx, "failing_job", nil)
	if err != nil {
		t.Fatalf("Enqueue failing_job failed: %v", err)
	}
	failJob := failJobResult.(*queue.Job)

	app.Queue().Dequeue(ctx) // Claim it

	err = app.Queue().Fail(ctx, failJob.ID, nil)
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}
}

// TestEventsPubSub tests the events system integration.
func TestEventsPubSub(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	ctx := context.Background()

	// Track received events
	var received []string
	var mu sync.Mutex

	// Subscribe to user.created events
	unsubscribe := app.Events().Subscribe("user.created", func(ctx context.Context, eventType string, payload any) {
		mu.Lock()
		defer mu.Unlock()
		received = append(received, eventType)
	})

	// Publish an event
	app.Events().Publish(ctx, "user.created", map[string]string{"email": "test@example.com"})

	// Check that event was received
	mu.Lock()
	if len(received) != 1 || received[0] != "user.created" {
		t.Errorf("expected [user.created], got %v", received)
	}
	mu.Unlock()

	// Unsubscribe and publish again
	unsubscribe()
	app.Events().Publish(ctx, "user.created", nil)

	// Should still be just 1 event (from before unsubscribe)
	mu.Lock()
	if len(received) != 1 {
		t.Errorf("expected 1 event after unsubscribe, got %d", len(received))
	}
	mu.Unlock()
}

// TestEmailSending tests email module with LogProvider.
func TestEmailSending(t *testing.T) {
	tmpDir := t.TempDir()

	// Track sent emails
	var sentEmails []struct {
		To, Subject, Body string
	}
	var emailMu sync.Mutex

	emailProvider := email.NewLogProvider(func(to, subject, body string) {
		emailMu.Lock()
		defer emailMu.Unlock()
		sentEmails = append(sentEmails, struct{ To, Subject, Body string }{to, subject, body})
	})

	app := chassis.New(
		chassis.WithModules(
			email.New(email.WithProvider(emailProvider)),
			// Minimal modules to avoid nil panics
			storage.New(storage.WithBasePath(filepath.Join(tmpDir, "storage"))),
		),
	)
	defer func() { _ = app.Shutdown(context.Background()) }()

	ctx := context.Background()

	// Send an email
	err := app.Email().Send(ctx, "user@example.com", "Welcome!", "Hello and welcome!")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	// Verify email was captured
	emailMu.Lock()
	defer emailMu.Unlock()

	if len(sentEmails) != 1 {
		t.Fatalf("expected 1 sent email, got %d", len(sentEmails))
	}
	if sentEmails[0].To != "user@example.com" {
		t.Errorf("to mismatch: got %q", sentEmails[0].To)
	}
	if sentEmails[0].Subject != "Welcome!" {
		t.Errorf("subject mismatch: got %q", sentEmails[0].Subject)
	}
}

// TestIntegratedUserOrgWorkflow tests a complete user workflow with all modules.
func TestIntegratedUserOrgWorkflow(t *testing.T) {
	tmpDir := t.TempDir()

	// Track events
	var eventLog []string
	var eventMu sync.Mutex

	// Track sent emails
	var sentEmails []string
	var emailMu sync.Mutex

	emailProvider := email.NewLogProvider(func(to, subject, body string) {
		emailMu.Lock()
		defer emailMu.Unlock()
		sentEmails = append(sentEmails, to)
	})

	app := chassis.New(
		chassis.WithModules(
			storage.New(storage.WithBasePath(filepath.Join(tmpDir, "storage"))),
			users.New(users.WithDBPath(filepath.Join(tmpDir, "users.db"))),
			auth.New(auth.WithDBPath(filepath.Join(tmpDir, "sessions.db"))),
			orgs.New(orgs.WithDBPath(filepath.Join(tmpDir, "orgs.db"))),
			permissions.New(),
			cache.New(),
			queue.New(queue.WithDBPath(filepath.Join(tmpDir, "queue.db"))),
			email.New(email.WithProvider(emailProvider)),
			events.New(),
		),
	)
	defer func() { _ = app.Shutdown(context.Background()) }()

	ctx := context.Background()

	// Set up event handlers
	app.Events().Subscribe("user.created", func(ctx context.Context, eventType string, payload any) {
		eventMu.Lock()
		defer eventMu.Unlock()
		eventLog = append(eventLog, "user.created")
	})

	app.Events().Subscribe("org.created", func(ctx context.Context, eventType string, payload any) {
		eventMu.Lock()
		defer eventMu.Unlock()
		eventLog = append(eventLog, "org.created")
	})

	// Step 1: Create a user
	userResult, err := app.Users().Create(ctx, "alice@example.com", "securepassword")
	if err != nil {
		t.Fatalf("create user failed: %v", err)
	}
	user := userResult.(*users.User)

	// Publish user created event
	app.Events().Publish(ctx, "user.created", map[string]string{"user_id": user.GetID()})

	// Step 2: Create an organization
	orgResult, err := app.Orgs().Create(ctx, orgs.CreateInput{Name: "Alice's Company"})
	if err != nil {
		t.Fatalf("create org failed: %v", err)
	}
	org := orgResult.(*orgs.Org)

	// Publish org created event
	app.Events().Publish(ctx, "org.created", map[string]string{"org_id": org.ID()})

	// Step 3: Add user as owner
	app.Orgs().AddMember(ctx, org.ID(), user.GetID(), "owner")

	// Step 4: Check permissions
	if !app.Permissions().Can(ctx, user.GetID(), "org:delete", org.ID()) {
		t.Error("owner should be able to delete org")
	}

	// Step 5: Store a file in org context
	docKey := "orgs/" + org.ID() + "/docs/readme.txt"
	app.Storage().Put(ctx, docKey, []byte("Welcome to Alice's Company"))

	// Step 6: Cache some data
	cacheKey := "org:" + org.ID() + ":settings"
	app.Cache().Set(ctx, cacheKey, []byte(`{"theme":"dark"}`))

	// Step 7: Enqueue a welcome email job
	app.Queue().Enqueue(ctx, "send_welcome_email", map[string]string{
		"user_id": user.GetID(),
		"org_id":  org.ID(),
	})

	// Step 8: Process the queue job
	jobResult, err := app.Queue().Dequeue(ctx)
	if err != nil {
		t.Fatalf("dequeue failed: %v", err)
	}
	job := jobResult.(*queue.Job)

	// Simulate processing: send email
	err = app.Email().Send(ctx, "alice@example.com", "Welcome to your company!", "You've created Alice's Company")
	if err != nil {
		t.Fatalf("send email failed: %v", err)
	}

	err = app.Queue().Complete(ctx, job.ID)
	if err != nil {
		t.Fatalf("complete job failed: %v", err)
	}

	// Verify all operations
	// Check events were logged
	eventMu.Lock()
	if len(eventLog) != 2 {
		t.Errorf("expected 2 events, got %d", len(eventLog))
	}
	eventMu.Unlock()

	// Check email was sent
	emailMu.Lock()
	if len(sentEmails) != 1 || sentEmails[0] != "alice@example.com" {
		t.Errorf("expected email to alice@example.com, got %v", sentEmails)
	}
	emailMu.Unlock()

	// Check storage
	data, err := app.Storage().Get(ctx, docKey)
	if err != nil || string(data) != "Welcome to Alice's Company" {
		t.Error("storage data mismatch")
	}

	// Check cache
	cached, found := app.Cache().Get(ctx, cacheKey)
	if !found || string(cached) != `{"theme":"dark"}` {
		t.Error("cache data mismatch")
	}
}

// TestConcurrentOperations tests thread safety across modules.
// Note: SQLite has limited concurrent write support, so this test focuses on
// concurrent reads and in-memory operations (cache, events) while doing
// sequential database writes.
func TestConcurrentOperations(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	ctx := context.Background()

	// First, create users sequentially (SQLite limitation)
	for i := 0; i < 5; i++ {
		email := fmt.Sprintf("user%d@example.com", i)
		_, err := app.Users().Create(ctx, email, "password123")
		if err != nil {
			t.Fatalf("failed to create user %d: %v", i, err)
		}
	}

	// Enqueue jobs sequentially (SQLite limitation)
	for i := 0; i < 5; i++ {
		_, err := app.Queue().Enqueue(ctx, "job_type", map[string]int{"n": i})
		if err != nil {
			t.Fatalf("failed to enqueue job %d: %v", i, err)
		}
	}

	var wg sync.WaitGroup
	errChan := make(chan error, 100)

	// Concurrent cache operations (in-memory, fully thread-safe)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key%d", n)
			err := app.Cache().Set(ctx, key, []byte("value"))
			if err != nil {
				errChan <- err
			}
		}(i)
	}

	// Concurrent cache reads
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("key%d", n%10)
			app.Cache().Get(ctx, key) // Just read, don't care about result
		}(i)
	}

	// Concurrent event publishing (in-memory, thread-safe)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			app.Events().PublishAsync(ctx, "test.event", n)
		}(i)
	}

	// Concurrent user reads (SQLite handles reads well)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			email := fmt.Sprintf("user%d@example.com", n%5)
			_, err := app.Users().GetByEmail(ctx, email)
			if err != nil {
				errChan <- err
			}
		}(i)
	}

	wg.Wait()
	close(errChan)

	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		t.Errorf("concurrent operations had errors: %v", errors)
	}
}

// TestShutdownCleansUp verifies that shutdown properly cleans up resources.
func TestShutdownCleansUp(t *testing.T) {
	tmpDir := t.TempDir()

	app := chassis.New(
		chassis.WithModules(
			storage.New(storage.WithBasePath(filepath.Join(tmpDir, "storage"))),
			users.New(users.WithDBPath(filepath.Join(tmpDir, "users.db"))),
			cache.New(),
			events.New(),
		),
	)

	ctx := context.Background()

	// Use the modules
	app.Storage().Put(ctx, "test.txt", []byte("test"))
	app.Cache().Set(ctx, "key", []byte("value"))

	// Subscribe to events
	app.Events().Subscribe("test", func(ctx context.Context, eventType string, payload any) {})

	// Shutdown
	err := app.Shutdown(ctx)
	if err != nil {
		t.Errorf("shutdown error: %v", err)
	}

	// After shutdown, accessing modules should work but operations may fail
	// This tests that shutdown is graceful
}

// TestMultipleOrgsPerUser tests a user belonging to multiple organizations.
func TestMultipleOrgsPerUser(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	ctx := context.Background()

	// Create a user
	userResult, _ := app.Users().Create(ctx, "multiorg@example.com", "password123")
	user := userResult.(*users.User)

	// Create multiple organizations
	org1Result, _ := app.Orgs().Create(ctx, orgs.CreateInput{Name: "Org 1"})
	org1 := org1Result.(*orgs.Org)

	org2Result, _ := app.Orgs().Create(ctx, orgs.CreateInput{Name: "Org 2"})
	org2 := org2Result.(*orgs.Org)

	org3Result, _ := app.Orgs().Create(ctx, orgs.CreateInput{Name: "Org 3"})
	org3 := org3Result.(*orgs.Org)

	// Add user to orgs with different roles
	app.Orgs().AddMember(ctx, org1.ID(), user.GetID(), "owner")
	app.Orgs().AddMember(ctx, org2.ID(), user.GetID(), "admin")
	app.Orgs().AddMember(ctx, org3.ID(), user.GetID(), "member")

	// Verify roles
	if role := app.Orgs().GetUserRole(ctx, org1.ID(), user.GetID()); role != "owner" {
		t.Errorf("expected owner in org1, got %q", role)
	}
	if role := app.Orgs().GetUserRole(ctx, org2.ID(), user.GetID()); role != "admin" {
		t.Errorf("expected admin in org2, got %q", role)
	}
	if role := app.Orgs().GetUserRole(ctx, org3.ID(), user.GetID()); role != "member" {
		t.Errorf("expected member in org3, got %q", role)
	}

	// Verify permissions differ by org
	if !app.Permissions().Can(ctx, user.GetID(), "org:delete", org1.ID()) {
		t.Error("owner should be able to delete org1")
	}
	if !app.Permissions().Can(ctx, user.GetID(), "org:delete", org2.ID()) {
		t.Error("admin should be able to delete org2")
	}
	if app.Permissions().Can(ctx, user.GetID(), "org:delete", org3.ID()) {
		t.Error("member should NOT be able to delete org3")
	}

	// Get all user orgs
	userOrgsResult, err := app.Orgs().GetUserOrgs(ctx, user.GetID())
	if err != nil {
		t.Fatalf("GetUserOrgs failed: %v", err)
	}
	userOrgs := userOrgsResult.([]*orgs.Membership)
	if len(userOrgs) != 3 {
		t.Errorf("expected 3 org memberships, got %d", len(userOrgs))
	}
}

// TestQueueWorkerPattern demonstrates the worker pattern (without actually running a worker).
func TestQueueWorkerPattern(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	ctx := context.Background()

	// Enqueue multiple jobs
	for i := 0; i < 5; i++ {
		app.Queue().Enqueue(ctx, "process_item", map[string]int{"item": i})
	}

	// Simulate worker processing
	processed := 0
	for {
		jobResult, err := app.Queue().Dequeue(ctx)
		if err != nil {
			if err == queue.ErrNoJobs {
				break
			}
			t.Fatalf("dequeue failed: %v", err)
		}

		job := jobResult.(*queue.Job)

		// "Process" the job
		err = app.Queue().Complete(ctx, job.ID)
		if err != nil {
			t.Fatalf("complete failed: %v", err)
		}
		processed++
	}

	if processed != 5 {
		t.Errorf("expected to process 5 jobs, got %d", processed)
	}
}

// TestEventsAsyncPublish tests async event publishing.
func TestEventsAsyncPublish(t *testing.T) {
	app, cleanup := setupTestApp(t)
	defer cleanup()

	ctx := context.Background()

	var received int32
	var mu sync.Mutex
	done := make(chan struct{})

	app.Events().Subscribe("async.test", func(ctx context.Context, eventType string, payload any) {
		mu.Lock()
		received++
		if received == 5 {
			close(done)
		}
		mu.Unlock()
	})

	// Publish async events
	for i := 0; i < 5; i++ {
		app.Events().PublishAsync(ctx, "async.test", i)
	}

	// Wait for all events to be processed (with timeout)
	select {
	case <-done:
		// All events received
	case <-time.After(2 * time.Second):
		mu.Lock()
		t.Errorf("timeout waiting for async events, received %d/5", received)
		mu.Unlock()
	}
}
