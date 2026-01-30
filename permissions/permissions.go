// Package permissions provides role-based access control (RBAC) for the chassis framework.
//
// It works in conjunction with the orgs module to check if users have
// specific permissions based on their role within an organization.
//
// # Usage
//
// Register the module with chassis:
//
//	app := chassis.New(
//	    chassis.WithModules(
//	        orgs.New(),        // Required: permissions checks org membership
//	        permissions.New(),
//	    ),
//	)
//
// Check permissions:
//
//	// Check if user can perform action on resource
//	if app.Permissions().Can(ctx, userID, "org:delete", orgID) {
//	    // User has permission
//	}
//
//	// Check if user has specific role
//	if app.Permissions().HasRole(ctx, userID, "admin", orgID) {
//	    // User is an admin
//	}
//
// # Default Permissions
//
// Built-in role permissions:
//
//	owner:  org:read, org:update, org:delete, org:manage_members, org:manage_roles
//	admin:  org:read, org:update, org:delete, org:manage_members
//	member: org:read
//
// # Custom Permissions
//
// Override default permissions:
//
//	permissions.New(permissions.WithRolePermissions(map[string][]string{
//	    "owner":  {"*"},  // Full access
//	    "editor": {"read", "write"},
//	    "viewer": {"read"},
//	}))
package permissions

import (
	"context"

	"github.com/talosaether/chassis"
)

// DefaultRolePermissions defines the default permission sets for each role.
var DefaultRolePermissions = map[string][]string{
	"owner": {
		"org:read",
		"org:update",
		"org:delete",
		"org:manage_members",
		"org:manage_roles",
	},
	"admin": {
		"org:read",
		"org:update",
		"org:delete",
		"org:manage_members",
	},
	"member": {
		"org:read",
	},
}

// Module is the permissions module implementation.
type Module struct {
	app             *chassis.App
	rolePermissions map[string]map[string]bool
}

// Option is a function that configures the permissions module.
type Option func(*Module)

// WithRolePermissions sets custom role-permission mappings.
func WithRolePermissions(rolePerms map[string][]string) Option {
	return func(mod *Module) {
		mod.rolePermissions = buildPermissionMap(rolePerms)
	}
}

// New creates a new permissions module with the given options.
func New(opts ...Option) *Module {
	mod := &Module{
		rolePermissions: buildPermissionMap(DefaultRolePermissions),
	}

	for _, opt := range opts {
		opt(mod)
	}

	return mod
}

func buildPermissionMap(rolePerms map[string][]string) map[string]map[string]bool {
	result := make(map[string]map[string]bool)
	for role, perms := range rolePerms {
		result[role] = make(map[string]bool)
		for _, perm := range perms {
			result[role][perm] = true
		}
	}
	return result
}

// Name returns the module identifier.
func (mod *Module) Name() string {
	return "permissions"
}

// Init initializes the permissions module.
func (mod *Module) Init(ctx context.Context, app *chassis.App) error {
	mod.app = app
	app.Logger().Info("permissions module initialized")
	return nil
}

// Shutdown cleans up the permissions module.
func (mod *Module) Shutdown(ctx context.Context) error {
	return nil
}

// Can checks if a user has a specific permission for a resource (typically an org ID).
func (mod *Module) Can(ctx context.Context, userID, permission, resourceID string) bool {
	orgsModule := mod.app.Orgs()
	userRole := orgsModule.GetUserRole(ctx, resourceID, userID)

	if userRole == "" {
		return false
	}

	return mod.RoleHasPermission(userRole, permission)
}

// RoleHasPermission checks if a role has a specific permission.
func (mod *Module) RoleHasPermission(role, permission string) bool {
	rolePerms, exists := mod.rolePermissions[role]
	if !exists {
		return false
	}
	return rolePerms[permission]
}

// GetRolePermissions returns all permissions for a given role.
func (mod *Module) GetRolePermissions(role string) []string {
	rolePerms, exists := mod.rolePermissions[role]
	if !exists {
		return nil
	}

	permissions := make([]string, 0, len(rolePerms))
	for perm := range rolePerms {
		permissions = append(permissions, perm)
	}
	return permissions
}

// GetAllRoles returns all defined roles.
func (mod *Module) GetAllRoles() []string {
	roles := make([]string, 0, len(mod.rolePermissions))
	for role := range mod.rolePermissions {
		roles = append(roles, role)
	}
	return roles
}

// HasRole checks if a user has a specific role in an organization.
func (mod *Module) HasRole(ctx context.Context, userID, role, resourceID string) bool {
	orgsModule := mod.app.Orgs()
	userRole := orgsModule.GetUserRole(ctx, resourceID, userID)
	return userRole == role
}

// HasAnyRole checks if a user has any of the specified roles in an organization.
func (mod *Module) HasAnyRole(ctx context.Context, userID string, roles []string, resourceID string) bool {
	orgsModule := mod.app.Orgs()
	userRole := orgsModule.GetUserRole(ctx, resourceID, userID)

	if userRole == "" {
		return false
	}

	for _, role := range roles {
		if userRole == role {
			return true
		}
	}
	return false
}
