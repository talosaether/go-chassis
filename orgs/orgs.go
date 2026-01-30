// Package orgs provides organization and team management for the chassis framework.
//
// It implements a multi-tenancy model where users can belong to multiple
// organizations with different roles. Organizations contain members, and
// each membership has an associated role that integrates with the
// permissions module.
//
// # Usage
//
// Register the module with chassis:
//
//	app := chassis.New(
//	    chassis.WithModules(
//	        orgs.New(),
//	    ),
//	)
//
// Create an organization and add members:
//
//	// Create org
//	org, err := app.Orgs().Create(ctx, orgs.CreateInput{Name: "Acme Corp"})
//
//	// Add member with role
//	membership, err := app.Orgs().AddMember(ctx, org.(*orgs.Org).ID(), userID, "admin")
//
//	// Get user's role
//	role := app.Orgs().GetUserRole(ctx, orgID, userID)
//
// # Roles
//
// The module supports three built-in roles: "owner", "admin", and "member".
// These integrate with the permissions module for access control.
//
// # Configuration
//
// Configure via config.yaml:
//
//	orgs:
//	  db_path: ./data/orgs.db
//
// Or programmatically:
//
//	orgs.New(orgs.WithDBPath("/custom/orgs.db"))
package orgs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/talosaether/chassis"
)

var (
	ErrNotFound       = errors.New("organization not found")
	ErrNameRequired   = errors.New("organization name is required")
	ErrNameExists     = errors.New("organization name already exists")
	ErrMemberNotFound = errors.New("member not found")
	ErrMemberExists   = errors.New("user is already a member of this organization")
	ErrInvalidRole    = errors.New("invalid role")
)

// ValidRoles defines the allowed membership roles.
var ValidRoles = map[string]bool{
	"owner":  true,
	"admin":  true,
	"member": true,
}

// Org represents an organization in the system.
type Org struct {
	id        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ID returns the organization's unique identifier.
func (org *Org) ID() string {
	return org.id
}

// Membership represents a user's membership in an organization.
type Membership struct {
	ID        string
	OrgID     string
	UserID    string
	Role      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CreateInput contains the data needed to create an organization.
type CreateInput struct {
	Name string
}

// UpdateInput contains the data that can be updated on an organization.
type UpdateInput struct {
	Name *string
}

// Module is the orgs module implementation.
type Module struct {
	store  Store
	dbPath string
	app    *chassis.App
}

// Option is a function that configures the orgs module.
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

// New creates a new orgs module with the given options.
func New(opts ...Option) *Module {
	mod := &Module{
		dbPath: "./data/orgs.db",
	}

	for _, opt := range opts {
		opt(mod)
	}

	return mod
}

// Name returns the module identifier.
func (mod *Module) Name() string {
	return "orgs"
}

// Init initializes the orgs module.
func (mod *Module) Init(ctx context.Context, app *chassis.App) error {
	mod.app = app

	// Read config if available
	if cfg := app.ConfigData(); cfg != nil {
		if dbPath := cfg.GetString("orgs.db_path"); dbPath != "" {
			mod.dbPath = dbPath
		}
	}

	// Use custom store if provided, otherwise create SQLite store
	if mod.store == nil {
		sqliteStore, err := NewSQLiteStore(mod.dbPath)
		if err != nil {
			return fmt.Errorf("failed to create orgs store: %w", err)
		}
		mod.store = sqliteStore
		app.Logger().Info("orgs module initialized", "db_path", mod.dbPath)
	} else {
		app.Logger().Info("orgs module initialized with custom store")
	}

	return nil
}

// Shutdown cleans up the orgs module.
func (mod *Module) Shutdown(ctx context.Context) error {
	if mod.store != nil {
		return mod.store.Close()
	}
	return nil
}

// Create creates a new organization.
func (mod *Module) Create(ctx context.Context, input any) (any, error) {
	createInput, ok := input.(CreateInput)
	if !ok {
		return nil, fmt.Errorf("invalid input type: expected CreateInput")
	}
	if createInput.Name == "" {
		return nil, ErrNameRequired
	}
	return mod.create(ctx, createInput)
}

// create is the internal implementation.
func (mod *Module) create(ctx context.Context, input CreateInput) (*Org, error) {
	if input.Name == "" {
		return nil, ErrNameRequired
	}

	// Check if org with this name already exists
	existing, err := mod.store.GetByName(ctx, input.Name)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("failed to check existing org: %w", err)
	}
	if existing != nil {
		return nil, ErrNameExists
	}

	now := time.Now()
	org := &Org{
		id:        uuid.New().String(),
		Name:      input.Name,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := mod.store.Create(ctx, org); err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	return org, nil
}

// GetByID retrieves an organization by its ID.
func (mod *Module) GetByID(ctx context.Context, orgID string) (any, error) {
	return mod.store.GetByID(ctx, orgID)
}

// Update updates an existing organization.
func (mod *Module) Update(ctx context.Context, orgID string, input any) (any, error) {
	updateInput, ok := input.(UpdateInput)
	if !ok {
		return nil, fmt.Errorf("invalid input type: expected UpdateInput")
	}
	return mod.update(ctx, orgID, updateInput)
}

// update is the internal implementation.
func (mod *Module) update(ctx context.Context, orgID string, input UpdateInput) (*Org, error) {
	org, err := mod.store.GetByID(ctx, orgID)
	if err != nil {
		return nil, err
	}

	if input.Name != nil {
		if *input.Name == "" {
			return nil, ErrNameRequired
		}
		org.Name = *input.Name
	}

	org.UpdatedAt = time.Now()

	if err := mod.store.Update(ctx, org); err != nil {
		return nil, fmt.Errorf("failed to update organization: %w", err)
	}

	return org, nil
}

// Delete removes an organization and all its memberships.
func (mod *Module) Delete(ctx context.Context, orgID string) error {
	if err := mod.store.DeleteMembershipsByOrgID(ctx, orgID); err != nil {
		return fmt.Errorf("failed to delete organization memberships: %w", err)
	}
	return mod.store.Delete(ctx, orgID)
}

// AddMember adds a user to an organization with the specified role.
func (mod *Module) AddMember(ctx context.Context, orgID, userID, role string) (any, error) {
	if !ValidRoles[role] {
		return nil, ErrInvalidRole
	}

	// Check if org exists
	_, err := mod.store.GetByID(ctx, orgID)
	if err != nil {
		return nil, err
	}

	// Check if already a member
	existing, err := mod.store.GetMembership(ctx, orgID, userID)
	if err != nil && !errors.Is(err, ErrMemberNotFound) {
		return nil, fmt.Errorf("failed to check existing membership: %w", err)
	}
	if existing != nil {
		return nil, ErrMemberExists
	}

	now := time.Now()
	membership := &Membership{
		ID:        uuid.New().String(),
		OrgID:     orgID,
		UserID:    userID,
		Role:      role,
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := mod.store.CreateMembership(ctx, membership); err != nil {
		return nil, fmt.Errorf("failed to add member: %w", err)
	}

	return membership, nil
}

// RemoveMember removes a user from an organization.
func (mod *Module) RemoveMember(ctx context.Context, orgID, userID string) error {
	return mod.store.DeleteMembership(ctx, orgID, userID)
}

// UpdateMemberRole updates a member's role in an organization.
func (mod *Module) UpdateMemberRole(ctx context.Context, orgID, userID, role string) (any, error) {
	if !ValidRoles[role] {
		return nil, ErrInvalidRole
	}

	membership, err := mod.store.GetMembership(ctx, orgID, userID)
	if err != nil {
		return nil, err
	}

	membership.Role = role
	membership.UpdatedAt = time.Now()

	if err := mod.store.UpdateMembership(ctx, membership); err != nil {
		return nil, fmt.Errorf("failed to update member role: %w", err)
	}

	return membership, nil
}

// GetMembers retrieves all members of an organization.
func (mod *Module) GetMembers(ctx context.Context, orgID string) (any, error) {
	_, err := mod.store.GetByID(ctx, orgID)
	if err != nil {
		return nil, err
	}
	return mod.store.GetMembersByOrgID(ctx, orgID)
}

// GetUserOrgs retrieves all organizations a user belongs to.
func (mod *Module) GetUserOrgs(ctx context.Context, userID string) (any, error) {
	return mod.store.GetMembershipsByUserID(ctx, userID)
}

// GetMembership retrieves a specific membership.
func (mod *Module) GetMembership(ctx context.Context, orgID, userID string) (any, error) {
	return mod.store.GetMembership(ctx, orgID, userID)
}

// GetUserRole returns the user's role in an organization, or empty string if not a member.
func (mod *Module) GetUserRole(ctx context.Context, orgID, userID string) string {
	membership, err := mod.store.GetMembership(ctx, orgID, userID)
	if err != nil {
		return ""
	}
	return membership.Role
}
