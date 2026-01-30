package users

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func setupTestStore(t *testing.T) (*SQLiteStore, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-users.db")

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

	user := &User{
		ID:           "test-user-id",
		Email:        "test@example.com",
		PasswordHash: "hash123",
	}

	err := store.Create(ctx, user)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := store.GetByID(ctx, "test-user-id")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if got.ID != user.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, user.ID)
	}
	if got.Email != user.Email {
		t.Errorf("Email mismatch: got %q, want %q", got.Email, user.Email)
	}
}

func TestSQLiteStore_GetByIDNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetByID(ctx, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestSQLiteStore_GetByEmail(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &User{
		ID:           "test-user-id",
		Email:        "test@example.com",
		PasswordHash: "hash123",
	}
	store.Create(ctx, user)

	got, err := store.GetByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("GetByEmail failed: %v", err)
	}

	if got.ID != user.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, user.ID)
	}
}

func TestSQLiteStore_GetByEmailNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetByEmail(ctx, "nonexistent@example.com")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestSQLiteStore_DuplicateEmail(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user1 := &User{ID: "user1", Email: "same@example.com", PasswordHash: "hash1"}
	user2 := &User{ID: "user2", Email: "same@example.com", PasswordHash: "hash2"}

	err := store.Create(ctx, user1)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	err = store.Create(ctx, user2)
	if err == nil {
		t.Fatal("second create should fail due to duplicate email")
	}
}

func TestSQLiteStore_Update(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &User{ID: "update-user", Email: "original@example.com", PasswordHash: "hash"}
	store.Create(ctx, user)

	user.Email = "updated@example.com"
	err := store.Update(ctx, user)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := store.GetByID(ctx, "update-user")
	if got.Email != "updated@example.com" {
		t.Errorf("Email should be 'updated@example.com', got %q", got.Email)
	}
}

func TestSQLiteStore_UpdateNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &User{ID: "nonexistent", Email: "test@example.com"}
	err := store.Update(ctx, user)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestSQLiteStore_Delete(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	user := &User{ID: "delete-user", Email: "delete@example.com", PasswordHash: "hash"}
	store.Create(ctx, user)

	err := store.Delete(ctx, "delete-user")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.GetByID(ctx, "delete-user")
	if !errors.Is(err, ErrNotFound) {
		t.Error("user should not exist after delete")
	}
}

func TestSQLiteStore_DeleteNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	err := store.Delete(ctx, "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

// Password hashing tests

func TestPasswordHashing(t *testing.T) {
	password := "testPassword123"

	hash, err := hashPassword(password)
	if err != nil {
		t.Fatalf("hashPassword failed: %v", err)
	}

	if hash == "" {
		t.Error("hash should not be empty")
	}
	if hash == password {
		t.Error("hash should not equal password")
	}
}

func TestPasswordVerification(t *testing.T) {
	password := "testPassword123"

	hash, _ := hashPassword(password)

	if !verifyPassword(password, hash) {
		t.Error("correct password should verify")
	}

	if verifyPassword("wrongPassword", hash) {
		t.Error("wrong password should not verify")
	}
}

func TestPasswordHashUniqueness(t *testing.T) {
	password := "samePassword"

	hash1, _ := hashPassword(password)
	hash2, _ := hashPassword(password)

	if hash1 == hash2 {
		t.Error("same password should produce different hashes (due to salt)")
	}

	// Both should still verify
	if !verifyPassword(password, hash1) {
		t.Error("password should verify against hash1")
	}
	if !verifyPassword(password, hash2) {
		t.Error("password should verify against hash2")
	}
}

func TestVerifyPasswordInvalidFormat(t *testing.T) {
	// Test with invalid hash formats
	invalidHashes := []string{
		"",
		"noDollarSign",
		"$onlyHash",
		"onlySalt$",
		"invalid$base64!!!",
	}

	for _, hash := range invalidHashes {
		if verifyPassword("password", hash) {
			t.Errorf("invalid hash %q should not verify", hash)
		}
	}
}

// Module tests

func TestModule_Name(t *testing.T) {
	mod := New()
	if mod.Name() != "users" {
		t.Errorf("Name() should return 'users', got %q", mod.Name())
	}
}

func TestModuleNew_DefaultOptions(t *testing.T) {
	mod := New()
	if mod.dbPath != "./data/users.db" {
		t.Errorf("default dbPath should be './data/users.db', got %q", mod.dbPath)
	}
}

func TestModuleNew_WithDBPath(t *testing.T) {
	mod := New(WithDBPath("/custom/users.db"))
	if mod.dbPath != "/custom/users.db" {
		t.Errorf("dbPath should be '/custom/users.db', got %q", mod.dbPath)
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

func TestModule_CreateValidation(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	// Empty email
	_, err := mod.Create(ctx, "", "password123")
	if !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("expected ErrInvalidEmail for empty email, got: %v", err)
	}

	// Weak password
	_, err = mod.Create(ctx, "test@example.com", "short")
	if !errors.Is(err, ErrWeakPassword) {
		t.Errorf("expected ErrWeakPassword for short password, got: %v", err)
	}
}

func TestModule_CreateSuccess(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	result, err := mod.Create(ctx, "test@example.com", "password123")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	user := result.(*User)
	if user.Email != "test@example.com" {
		t.Errorf("email should be 'test@example.com', got %q", user.Email)
	}
	if user.ID == "" {
		t.Error("ID should be generated")
	}
	if user.PasswordHash == "" {
		t.Error("PasswordHash should be set")
	}
}

func TestModule_CreateDuplicateEmail(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	mod.Create(ctx, "duplicate@example.com", "password123")

	_, err := mod.Create(ctx, "duplicate@example.com", "password456")
	if !errors.Is(err, ErrEmailExists) {
		t.Errorf("expected ErrEmailExists, got: %v", err)
	}
}

func TestModule_Authenticate(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	password := "password123"
	mod.Create(ctx, "auth@example.com", password)

	// Correct password
	result, err := mod.Authenticate(ctx, "auth@example.com", password)
	if err != nil {
		t.Fatalf("Authenticate failed: %v", err)
	}

	user := result.(*User)
	if user.Email != "auth@example.com" {
		t.Errorf("email mismatch: got %q", user.Email)
	}
}

func TestModule_AuthenticateWrongPassword(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	mod.Create(ctx, "auth@example.com", "password123")

	_, err := mod.Authenticate(ctx, "auth@example.com", "wrongpassword")
	if !errors.Is(err, ErrWrongPassword) {
		t.Errorf("expected ErrWrongPassword, got: %v", err)
	}
}

func TestModule_AuthenticateNonexistentUser(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	_, err := mod.Authenticate(ctx, "nonexistent@example.com", "password123")
	if !errors.Is(err, ErrWrongPassword) {
		// Should return ErrWrongPassword to avoid revealing email existence
		t.Errorf("expected ErrWrongPassword, got: %v", err)
	}
}

func TestModule_GetByID(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	createResult, _ := mod.Create(ctx, "test@example.com", "password123")
	created := createResult.(*User)

	result, err := mod.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	user := result.(*User)
	if user.ID != created.ID {
		t.Errorf("ID mismatch: got %q, want %q", user.ID, created.ID)
	}
}

func TestModule_GetByEmail(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	mod.Create(ctx, "test@example.com", "password123")

	result, err := mod.GetByEmail(ctx, "test@example.com")
	if err != nil {
		t.Fatalf("GetByEmail failed: %v", err)
	}

	user := result.(*User)
	if user.Email != "test@example.com" {
		t.Errorf("email mismatch: got %q", user.Email)
	}
}

func TestModule_Update(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	createResult, _ := mod.Create(ctx, "original@example.com", "password123")
	created := createResult.(*User)

	newEmail := "updated@example.com"
	user, err := mod.Update(ctx, created.ID, UpdateInput{
		Email: &newEmail,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if user.Email != newEmail {
		t.Errorf("email should be %q, got %q", newEmail, user.Email)
	}
}

func TestModule_UpdatePassword(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	createResult, _ := mod.Create(ctx, "test@example.com", "oldPassword123")
	created := createResult.(*User)

	newPassword := "newPassword123"
	_, err := mod.Update(ctx, created.ID, UpdateInput{
		Password: &newPassword,
	})
	if err != nil {
		t.Fatalf("Update password failed: %v", err)
	}

	// Should authenticate with new password
	_, err = mod.Authenticate(ctx, "test@example.com", newPassword)
	if err != nil {
		t.Error("should authenticate with new password")
	}

	// Should not authenticate with old password
	_, err = mod.Authenticate(ctx, "test@example.com", "oldPassword123")
	if !errors.Is(err, ErrWrongPassword) {
		t.Error("should not authenticate with old password")
	}
}

func TestModule_Delete(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	mod := New(WithStore(store))
	ctx := context.Background()

	createResult, _ := mod.Create(ctx, "test@example.com", "password123")
	created := createResult.(*User)

	err := mod.Delete(ctx, created.ID)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = mod.GetByID(ctx, created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Error("user should not exist after delete")
	}
}

func TestUser_GetID(t *testing.T) {
	user := &User{ID: "test-id"}
	if user.GetID() != "test-id" {
		t.Errorf("GetID() should return 'test-id', got %q", user.GetID())
	}
}

func TestUser_GetEmail(t *testing.T) {
	user := &User{Email: "test@example.com"}
	if user.GetEmail() != "test@example.com" {
		t.Errorf("GetEmail() should return 'test@example.com', got %q", user.GetEmail())
	}
}
