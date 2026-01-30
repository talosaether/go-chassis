package auth

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func setupTestStore(t *testing.T) (*SQLiteSessionStore, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-sessions.db")

	store, err := NewSQLiteSessionStore(dbPath)
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}

	cleanup := func() {
		_ = store.Close()
		_ = os.Remove(dbPath)
	}

	return store, cleanup
}

func TestSQLiteSessionStore_CreateAndGetByID(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session := &Session{
		ID:        "test-session-id",
		UserID:    "test-user-id",
		Token:     "test-token-123",
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}

	err := store.Create(ctx, session)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := store.GetByID(ctx, "test-session-id")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if got.ID != session.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, session.ID)
	}
	if got.UserID != session.UserID {
		t.Errorf("UserID mismatch: got %q, want %q", got.UserID, session.UserID)
	}
	if got.Token != session.Token {
		t.Errorf("Token mismatch: got %q, want %q", got.Token, session.Token)
	}
}

func TestSQLiteSessionStore_GetByIDNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetByID(ctx, "nonexistent")
	if !errors.Is(err, ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession, got: %v", err)
	}
}

func TestSQLiteSessionStore_GetByToken(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session := &Session{
		ID:        "test-session-id",
		UserID:    "test-user-id",
		Token:     "unique-token-xyz",
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}
	store.Create(ctx, session)

	got, err := store.GetByToken(ctx, "unique-token-xyz")
	if err != nil {
		t.Fatalf("GetByToken failed: %v", err)
	}

	if got.ID != session.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, session.ID)
	}
}

func TestSQLiteSessionStore_GetByTokenNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetByToken(ctx, "nonexistent-token")
	if !errors.Is(err, ErrInvalidSession) {
		t.Errorf("expected ErrInvalidSession, got: %v", err)
	}
}

func TestSQLiteSessionStore_DuplicateToken(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session1 := &Session{ID: "session1", UserID: "user1", Token: "same-token", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()}
	session2 := &Session{ID: "session2", UserID: "user2", Token: "same-token", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()}

	err := store.Create(ctx, session1)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	err = store.Create(ctx, session2)
	if err == nil {
		t.Fatal("second create should fail due to duplicate token")
	}
}

func TestSQLiteSessionStore_Delete(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session := &Session{
		ID:        "to-delete",
		UserID:    "user-id",
		Token:     "token-123",
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}
	store.Create(ctx, session)

	err := store.Delete(ctx, "to-delete")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.GetByID(ctx, "to-delete")
	if !errors.Is(err, ErrInvalidSession) {
		t.Error("session should not exist after delete")
	}
}

func TestSQLiteSessionStore_DeleteByToken(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	session := &Session{
		ID:        "session-id",
		UserID:    "user-id",
		Token:     "token-to-delete",
		ExpiresAt: time.Now().Add(time.Hour),
		CreatedAt: time.Now(),
	}
	store.Create(ctx, session)

	err := store.DeleteByToken(ctx, "token-to-delete")
	if err != nil {
		t.Fatalf("DeleteByToken failed: %v", err)
	}

	_, err = store.GetByToken(ctx, "token-to-delete")
	if !errors.Is(err, ErrInvalidSession) {
		t.Error("session should not exist after DeleteByToken")
	}
}

func TestSQLiteSessionStore_DeleteByUserID(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple sessions for the same user
	store.Create(ctx, &Session{ID: "s1", UserID: "user1", Token: "t1", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()})
	store.Create(ctx, &Session{ID: "s2", UserID: "user1", Token: "t2", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()})
	store.Create(ctx, &Session{ID: "s3", UserID: "user2", Token: "t3", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now()})

	err := store.DeleteByUserID(ctx, "user1")
	if err != nil {
		t.Fatalf("DeleteByUserID failed: %v", err)
	}

	// user1's sessions should be gone
	_, err = store.GetByID(ctx, "s1")
	if !errors.Is(err, ErrInvalidSession) {
		t.Error("session s1 should be deleted")
	}
	_, err = store.GetByID(ctx, "s2")
	if !errors.Is(err, ErrInvalidSession) {
		t.Error("session s2 should be deleted")
	}

	// user2's session should still exist
	_, err = store.GetByID(ctx, "s3")
	if err != nil {
		t.Error("session s3 should still exist")
	}
}

// Module tests

func TestModule_Name(t *testing.T) {
	mod := New()
	if mod.Name() != "auth" {
		t.Errorf("Name() should return 'auth', got %q", mod.Name())
	}
}

func TestModuleNew_DefaultOptions(t *testing.T) {
	mod := New()

	if mod.dbPath != "./data/sessions.db" {
		t.Errorf("default dbPath should be './data/sessions.db', got %q", mod.dbPath)
	}
	if mod.cookieName != "session" {
		t.Errorf("default cookieName should be 'session', got %q", mod.cookieName)
	}
	if mod.sessionTTL != 24*time.Hour {
		t.Errorf("default sessionTTL should be 24h, got %v", mod.sessionTTL)
	}
	if mod.secureCookie != false {
		t.Error("default secureCookie should be false")
	}
}

func TestModuleNew_WithOptions(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(
		WithStore(store),
		WithDBPath("/custom/sessions.db"),
		WithCookieName("my_session"),
		WithSessionTTL(48*time.Hour),
		WithSecureCookie(true),
	)

	if mod.store != store {
		t.Error("custom store should be set")
	}
	if mod.dbPath != "/custom/sessions.db" {
		t.Errorf("dbPath should be '/custom/sessions.db', got %q", mod.dbPath)
	}
	if mod.cookieName != "my_session" {
		t.Errorf("cookieName should be 'my_session', got %q", mod.cookieName)
	}
	if mod.sessionTTL != 48*time.Hour {
		t.Errorf("sessionTTL should be 48h, got %v", mod.sessionTTL)
	}
	if mod.secureCookie != true {
		t.Error("secureCookie should be true")
	}
}

// Context helper tests

func TestSessionFromContext(t *testing.T) {
	session := &Session{
		ID:     "ctx-session",
		UserID: "ctx-user",
		Token:  "ctx-token",
	}

	ctx := context.WithValue(context.Background(), sessionContextKey, session)

	got := SessionFromContext(ctx)
	if got == nil {
		t.Fatal("SessionFromContext should return session")
	}
	if got.ID != session.ID {
		t.Errorf("session ID mismatch: got %q, want %q", got.ID, session.ID)
	}
}

func TestSessionFromContext_NoSession(t *testing.T) {
	ctx := context.Background()

	got := SessionFromContext(ctx)
	if got != nil {
		t.Error("SessionFromContext should return nil when no session in context")
	}
}

func TestUserIDFromContext(t *testing.T) {
	session := &Session{
		ID:     "ctx-session",
		UserID: "ctx-user-id",
		Token:  "ctx-token",
	}

	ctx := context.WithValue(context.Background(), sessionContextKey, session)

	got := UserIDFromContext(ctx)
	if got != "ctx-user-id" {
		t.Errorf("UserIDFromContext should return 'ctx-user-id', got %q", got)
	}
}

func TestUserIDFromContext_NoSession(t *testing.T) {
	ctx := context.Background()

	got := UserIDFromContext(ctx)
	if got != "" {
		t.Errorf("UserIDFromContext should return empty string when no session, got %q", got)
	}
}

// Token generation tests

func TestGenerateToken(t *testing.T) {
	token1, err := generateToken(32)
	if err != nil {
		t.Fatalf("generateToken failed: %v", err)
	}

	if token1 == "" {
		t.Error("token should not be empty")
	}

	// Generate another token to verify uniqueness
	token2, _ := generateToken(32)
	if token1 == token2 {
		t.Error("tokens should be unique")
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID()
	id2 := generateID()

	if id1 == "" {
		t.Error("ID should not be empty")
	}
	if id1 == id2 {
		t.Error("IDs should be unique")
	}
}

// Error definitions test

func TestErrors(t *testing.T) {
	if ErrInvalidSession.Error() != "invalid or expired session" {
		t.Errorf("ErrInvalidSession message wrong: %q", ErrInvalidSession.Error())
	}
	if ErrNotAuthenticated.Error() != "not authenticated" {
		t.Errorf("ErrNotAuthenticated message wrong: %q", ErrNotAuthenticated.Error())
	}
}
