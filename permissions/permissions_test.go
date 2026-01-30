package permissions

import (
	"sort"
	"testing"
)

func TestModule_Name(t *testing.T) {
	mod := New()
	if mod.Name() != "permissions" {
		t.Errorf("Name() should return 'permissions', got %q", mod.Name())
	}
}

func TestModule_RoleHasPermission(t *testing.T) {
	mod := New()

	tests := []struct {
		role       string
		permission string
		expected   bool
	}{
		// Owner permissions
		{"owner", "org:read", true},
		{"owner", "org:update", true},
		{"owner", "org:delete", true},
		{"owner", "org:manage_members", true},
		{"owner", "org:manage_roles", true},

		// Admin permissions
		{"admin", "org:read", true},
		{"admin", "org:update", true},
		{"admin", "org:delete", true},
		{"admin", "org:manage_members", true},
		{"admin", "org:manage_roles", false}, // Admin cannot manage roles

		// Member permissions
		{"member", "org:read", true},
		{"member", "org:update", false},
		{"member", "org:delete", false},
		{"member", "org:manage_members", false},

		// Invalid role
		{"invalid", "org:read", false},

		// Invalid permission
		{"owner", "invalid:permission", false},
	}

	for _, tt := range tests {
		t.Run(tt.role+"_"+tt.permission, func(t *testing.T) {
			got := mod.RoleHasPermission(tt.role, tt.permission)
			if got != tt.expected {
				t.Errorf("RoleHasPermission(%q, %q) = %v, want %v",
					tt.role, tt.permission, got, tt.expected)
			}
		})
	}
}

func TestModule_GetRolePermissions(t *testing.T) {
	mod := New()

	// Test owner permissions
	ownerPerms := mod.GetRolePermissions("owner")
	if len(ownerPerms) != 5 {
		t.Errorf("owner should have 5 permissions, got %d", len(ownerPerms))
	}

	// Test member permissions
	memberPerms := mod.GetRolePermissions("member")
	if len(memberPerms) != 1 {
		t.Errorf("member should have 1 permission, got %d", len(memberPerms))
	}

	// Test invalid role
	invalidPerms := mod.GetRolePermissions("invalid")
	if invalidPerms != nil {
		t.Error("invalid role should return nil permissions")
	}
}

func TestModule_GetAllRoles(t *testing.T) {
	mod := New()

	roles := mod.GetAllRoles()

	// Should have owner, admin, member
	if len(roles) != 3 {
		t.Errorf("expected 3 roles, got %d", len(roles))
	}

	// Check all expected roles are present
	sort.Strings(roles)
	expected := []string{"admin", "member", "owner"}
	for i, role := range expected {
		if roles[i] != role {
			t.Errorf("expected role %q at position %d, got %q", role, i, roles[i])
		}
	}
}

func TestModule_WithRolePermissions(t *testing.T) {
	customPerms := map[string][]string{
		"super_admin": {"*"},
		"viewer":      {"read"},
	}

	mod := New(WithRolePermissions(customPerms))

	// Check custom roles work
	if !mod.RoleHasPermission("super_admin", "*") {
		t.Error("super_admin should have * permission")
	}
	if !mod.RoleHasPermission("viewer", "read") {
		t.Error("viewer should have read permission")
	}

	// Default roles should not exist
	if mod.RoleHasPermission("owner", "org:read") {
		t.Error("owner role should not exist with custom permissions")
	}
}

func TestDefaultRolePermissions(t *testing.T) {
	// Verify the default role permissions are what we expect
	if len(DefaultRolePermissions) != 3 {
		t.Errorf("expected 3 default roles, got %d", len(DefaultRolePermissions))
	}

	if _, exists := DefaultRolePermissions["owner"]; !exists {
		t.Error("owner role should exist in defaults")
	}
	if _, exists := DefaultRolePermissions["admin"]; !exists {
		t.Error("admin role should exist in defaults")
	}
	if _, exists := DefaultRolePermissions["member"]; !exists {
		t.Error("member role should exist in defaults")
	}
}

func TestBuildPermissionMap(t *testing.T) {
	input := map[string][]string{
		"role1": {"perm1", "perm2"},
		"role2": {"perm3"},
	}

	result := buildPermissionMap(input)

	if len(result) != 2 {
		t.Errorf("expected 2 roles, got %d", len(result))
	}

	if !result["role1"]["perm1"] {
		t.Error("role1 should have perm1")
	}
	if !result["role1"]["perm2"] {
		t.Error("role1 should have perm2")
	}
	if !result["role2"]["perm3"] {
		t.Error("role2 should have perm3")
	}
	if result["role1"]["perm3"] {
		t.Error("role1 should not have perm3")
	}
}
