package orgs

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func setupTestStore(t *testing.T) (*SQLiteStore, func()) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test-orgs.db")

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

	org := &Org{
		id:   "test-org-id",
		Name: "Test Org",
	}

	err := store.Create(ctx, org)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, err := store.GetByID(ctx, "test-org-id")
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}

	if got.ID() != org.id {
		t.Errorf("ID mismatch: got %q, want %q", got.ID(), org.id)
	}
	if got.Name != org.Name {
		t.Errorf("Name mismatch: got %q, want %q", got.Name, org.Name)
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

func TestSQLiteStore_GetByName(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	org := &Org{
		id:   "test-org-id",
		Name: "Unique Org Name",
	}
	store.Create(ctx, org)

	got, err := store.GetByName(ctx, "Unique Org Name")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}

	if got.ID() != org.id {
		t.Errorf("ID mismatch: got %q, want %q", got.ID(), org.id)
	}
}

func TestSQLiteStore_GetByNameNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetByName(ctx, "Nonexistent Org")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestSQLiteStore_DuplicateName(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	org1 := &Org{id: "org1", Name: "Same Name"}
	org2 := &Org{id: "org2", Name: "Same Name"}

	err := store.Create(ctx, org1)
	if err != nil {
		t.Fatalf("first create failed: %v", err)
	}

	err = store.Create(ctx, org2)
	if err == nil {
		t.Fatal("second create should fail due to duplicate name")
	}
}

func TestSQLiteStore_Update(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	org := &Org{id: "update-org", Name: "Original Name"}
	store.Create(ctx, org)

	org.Name = "Updated Name"
	err := store.Update(ctx, org)
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, _ := store.GetByID(ctx, "update-org")
	if got.Name != "Updated Name" {
		t.Errorf("Name should be 'Updated Name', got %q", got.Name)
	}
}

func TestSQLiteStore_UpdateNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	org := &Org{id: "nonexistent", Name: "Name"}
	err := store.Update(ctx, org)
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got: %v", err)
	}
}

func TestSQLiteStore_Delete(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	org := &Org{id: "delete-org", Name: "To Delete"}
	store.Create(ctx, org)

	err := store.Delete(ctx, "delete-org")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = store.GetByID(ctx, "delete-org")
	if !errors.Is(err, ErrNotFound) {
		t.Error("org should not exist after delete")
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

// Membership tests

func TestSQLiteStore_CreateAndGetMembership(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create org first
	org := &Org{id: "org-id", Name: "Org"}
	store.Create(ctx, org)

	membership := &Membership{
		ID:     "mem-id",
		OrgID:  "org-id",
		UserID: "user-id",
		Role:   "member",
	}
	store.CreateMembership(ctx, membership)

	got, err := store.GetMembership(ctx, "org-id", "user-id")
	if err != nil {
		t.Fatalf("GetMembership failed: %v", err)
	}

	if got.ID != membership.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, membership.ID)
	}
	if got.Role != "member" {
		t.Errorf("Role should be 'member', got %q", got.Role)
	}
}

func TestSQLiteStore_GetMembershipNotFound(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	_, err := store.GetMembership(ctx, "org-id", "user-id")
	if !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("expected ErrMemberNotFound, got: %v", err)
	}
}

func TestSQLiteStore_DuplicateMembership(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	org := &Org{id: "org-id", Name: "Org"}
	store.Create(ctx, org)

	mem1 := &Membership{ID: "mem1", OrgID: "org-id", UserID: "user-id", Role: "member"}
	mem2 := &Membership{ID: "mem2", OrgID: "org-id", UserID: "user-id", Role: "admin"}

	store.CreateMembership(ctx, mem1)
	err := store.CreateMembership(ctx, mem2)
	if err == nil {
		t.Fatal("duplicate membership should fail")
	}
}

func TestSQLiteStore_GetMembersByOrgID(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	org := &Org{id: "org-id", Name: "Org"}
	store.Create(ctx, org)

	store.CreateMembership(ctx, &Membership{ID: "m1", OrgID: "org-id", UserID: "user1", Role: "owner"})
	store.CreateMembership(ctx, &Membership{ID: "m2", OrgID: "org-id", UserID: "user2", Role: "member"})
	store.CreateMembership(ctx, &Membership{ID: "m3", OrgID: "org-id", UserID: "user3", Role: "member"})

	members, err := store.GetMembersByOrgID(ctx, "org-id")
	if err != nil {
		t.Fatalf("GetMembersByOrgID failed: %v", err)
	}

	if len(members) != 3 {
		t.Errorf("expected 3 members, got %d", len(members))
	}
}

func TestSQLiteStore_GetMembershipsByUserID(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	// Create multiple orgs
	store.Create(ctx, &Org{id: "org1", Name: "Org1"})
	store.Create(ctx, &Org{id: "org2", Name: "Org2"})

	store.CreateMembership(ctx, &Membership{ID: "m1", OrgID: "org1", UserID: "user1", Role: "member"})
	store.CreateMembership(ctx, &Membership{ID: "m2", OrgID: "org2", UserID: "user1", Role: "admin"})

	memberships, err := store.GetMembershipsByUserID(ctx, "user1")
	if err != nil {
		t.Fatalf("GetMembershipsByUserID failed: %v", err)
	}

	if len(memberships) != 2 {
		t.Errorf("expected 2 memberships, got %d", len(memberships))
	}
}

func TestSQLiteStore_UpdateMembership(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	org := &Org{id: "org-id", Name: "Org"}
	store.Create(ctx, org)

	membership := &Membership{ID: "mem-id", OrgID: "org-id", UserID: "user-id", Role: "member"}
	store.CreateMembership(ctx, membership)

	membership.Role = "admin"
	err := store.UpdateMembership(ctx, membership)
	if err != nil {
		t.Fatalf("UpdateMembership failed: %v", err)
	}

	got, _ := store.GetMembership(ctx, "org-id", "user-id")
	if got.Role != "admin" {
		t.Errorf("Role should be 'admin', got %q", got.Role)
	}
}

func TestSQLiteStore_DeleteMembership(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	org := &Org{id: "org-id", Name: "Org"}
	store.Create(ctx, org)

	membership := &Membership{ID: "mem-id", OrgID: "org-id", UserID: "user-id", Role: "member"}
	store.CreateMembership(ctx, membership)

	err := store.DeleteMembership(ctx, "org-id", "user-id")
	if err != nil {
		t.Fatalf("DeleteMembership failed: %v", err)
	}

	_, err = store.GetMembership(ctx, "org-id", "user-id")
	if !errors.Is(err, ErrMemberNotFound) {
		t.Error("membership should not exist after delete")
	}
}

func TestSQLiteStore_DeleteMembershipsByOrgID(t *testing.T) {
	store, cleanup := setupTestStore(t)
	defer cleanup()

	ctx := context.Background()

	org := &Org{id: "org-id", Name: "Org"}
	store.Create(ctx, org)

	store.CreateMembership(ctx, &Membership{ID: "m1", OrgID: "org-id", UserID: "user1", Role: "member"})
	store.CreateMembership(ctx, &Membership{ID: "m2", OrgID: "org-id", UserID: "user2", Role: "member"})

	err := store.DeleteMembershipsByOrgID(ctx, "org-id")
	if err != nil {
		t.Fatalf("DeleteMembershipsByOrgID failed: %v", err)
	}

	members, _ := store.GetMembersByOrgID(ctx, "org-id")
	if len(members) != 0 {
		t.Errorf("expected 0 members after delete, got %d", len(members))
	}
}

// Module tests

func TestModule_Name(t *testing.T) {
	mod := New()
	if mod.Name() != "orgs" {
		t.Errorf("Name() should return 'orgs', got %q", mod.Name())
	}
}

func TestModuleNew_DefaultOptions(t *testing.T) {
	mod := New()
	if mod.dbPath != "./data/orgs.db" {
		t.Errorf("default dbPath should be './data/orgs.db', got %q", mod.dbPath)
	}
}

func TestModuleNew_WithDBPath(t *testing.T) {
	mod := New(WithDBPath("/custom/orgs.db"))
	if mod.dbPath != "/custom/orgs.db" {
		t.Errorf("dbPath should be '/custom/orgs.db', got %q", mod.dbPath)
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

func TestValidRoles(t *testing.T) {
	validRoles := []string{"owner", "admin", "member"}
	for _, role := range validRoles {
		if !ValidRoles[role] {
			t.Errorf("%q should be a valid role", role)
		}
	}

	if ValidRoles["invalid-role"] {
		t.Error("'invalid-role' should not be valid")
	}
}
