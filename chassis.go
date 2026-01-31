package chassis

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// App is the central chassis instance that holds all registered modules.
// It manages lifecycle and provides access to module APIs.
type App struct {
	mu         sync.RWMutex
	modules    map[string]Module
	config     *Config
	configData ConfigData
	logger     *slog.Logger

	// Module accessors (populated during registration)
	storage     StorageModule
	users       UsersModule
	auth        AuthModule
	orgs        OrgsModule
	permissions PermissionsModule
	cache       CacheModule
	queue       QueueModule
	email       EmailModule
	events      EventsModule
}

// StorageModule is the interface exposed by the storage module.
// Defined here so the App can hold a typed reference.
type StorageModule interface {
	Module
	Put(ctx context.Context, key string, data []byte) error
	Get(ctx context.Context, key string) ([]byte, error)
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// UsersModule is the interface exposed by the users module.
// Methods return any to avoid import cycles; callers should use the users package types.
type UsersModule interface {
	Module
	Create(ctx context.Context, email, password string) (any, error)
	GetByID(ctx context.Context, id string) (any, error)
	GetByEmail(ctx context.Context, email string) (any, error)
	Authenticate(ctx context.Context, email, password string) (any, error)
}

// AuthModule is the interface exposed by the auth module.
type AuthModule interface {
	Module
	GetUserID(ctx context.Context, request any) string
}

// OrgsModule is the interface exposed by the orgs module.
type OrgsModule interface {
	Module
	Create(ctx context.Context, input any) (any, error)
	GetByID(ctx context.Context, orgID string) (any, error)
	Update(ctx context.Context, orgID string, input any) (any, error)
	Delete(ctx context.Context, orgID string) error
	AddMember(ctx context.Context, orgID, userID, role string) (any, error)
	RemoveMember(ctx context.Context, orgID, userID string) error
	GetMembers(ctx context.Context, orgID string) (any, error)
	GetUserOrgs(ctx context.Context, userID string) (any, error)
	GetUserRole(ctx context.Context, orgID, userID string) string
}

// PermissionsModule is the interface exposed by the permissions module.
type PermissionsModule interface {
	Module
	Can(ctx context.Context, userID, permission, resourceID string) bool
	RoleHasPermission(role, permission string) bool
	HasRole(ctx context.Context, userID, role, resourceID string) bool
}

// CacheModule is the interface exposed by the cache module.
type CacheModule interface {
	Module
	Get(ctx context.Context, key string) ([]byte, bool)
	Set(ctx context.Context, key string, value []byte) error
	Delete(ctx context.Context, key string) error
	Clear(ctx context.Context) error
}

// QueueModule is the interface exposed by the queue module.
type QueueModule interface {
	Module
	Enqueue(ctx context.Context, jobType string, payload any) (any, error)
	Dequeue(ctx context.Context) (any, error)
	Complete(ctx context.Context, jobID string) error
	Fail(ctx context.Context, jobID string, err error) error
	GetByID(ctx context.Context, jobID string) (any, error)
	GetAll(ctx context.Context) (any, error)
	GetPending(ctx context.Context) (any, error)
	GetCompleted(ctx context.Context) (any, error)
	GetFailed(ctx context.Context) (any, error)
}

// EmailModule is the interface exposed by the email module.
type EmailModule interface {
	Module
	Send(ctx context.Context, to, subject, body string) error
}

// EventsModule is the interface exposed by the events module.
type EventsModule interface {
	Module
	Subscribe(eventType string, handler any) func()
	Publish(ctx context.Context, eventType string, payload any)
	PublishAsync(ctx context.Context, eventType string, payload any)
}

// Config holds chassis configuration.
// Will be expanded to support YAML loading, env vars, etc.
type Config struct {
	Env      string
	LogLevel slog.Level
}

// Option configures the App during creation.
type Option func(*App)

// New creates a new chassis App with the given options.
func New(opts ...Option) *App {
	app := &App{
		modules: make(map[string]Module),
		config: &Config{
			Env:      "development",
			LogLevel: slog.LevelInfo,
		},
		logger: slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})),
	}

	for _, opt := range opts {
		opt(app)
	}

	return app
}

// WithConfig sets configuration options.
func WithConfig(cfg *Config) Option {
	return func(app *App) {
		app.config = cfg
		// Update logger level if config specifies it
		app.logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: cfg.LogLevel,
		}))
	}
}

// WithConfigFile loads configuration from a YAML file.
// Environment variables in ${VAR} or ${VAR:-default} format are expanded.
func WithConfigFile(path string) Option {
	return func(app *App) {
		data, err := LoadConfig(path)
		if err != nil {
			app.logger.Error("failed to load config file", "path", path, "error", err)
			return
		}
		app.configData = data

		// Apply chassis-level config if present
		if chassisSection := data.Section("chassis"); chassisSection != nil {
			if env := chassisSection.GetString("env"); env != "" {
				app.config.Env = env
			}
			if logLevel := chassisSection.GetString("log_level"); logLevel != "" {
				var level slog.Level
				if err := level.UnmarshalText([]byte(logLevel)); err == nil {
					app.config.LogLevel = level
					app.logger = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
						Level: level,
					}))
				}
			}
		}

		app.logger.Info("config loaded", "path", path)
	}
}

// WithModules registers modules with the chassis.
// Modules are initialized in the order provided.
func WithModules(modules ...Module) Option {
	return func(app *App) {
		ctx := context.Background()
		for _, mod := range modules {
			if err := app.Register(ctx, mod); err != nil {
				app.logger.Error("failed to register module",
					"module", mod.Name(),
					"error", err,
				)
			}
		}
	}
}

// Register adds a module to the chassis and initializes it.
func (app *App) Register(ctx context.Context, mod Module) error {
	app.mu.Lock()
	defer app.mu.Unlock()

	name := mod.Name()
	if _, exists := app.modules[name]; exists {
		return fmt.Errorf("module %q already registered", name)
	}

	// Initialize the module
	if err := mod.Init(ctx, app); err != nil {
		return fmt.Errorf("failed to initialize module %q: %w", name, err)
	}

	app.modules[name] = mod
	app.logger.Info("module registered", "module", name)

	// Wire up typed accessors for known modules
	if storageMod, ok := mod.(StorageModule); ok {
		app.storage = storageMod
	}
	if usersMod, ok := mod.(UsersModule); ok {
		app.users = usersMod
	}
	if authMod, ok := mod.(AuthModule); ok {
		app.auth = authMod
	}
	if orgsMod, ok := mod.(OrgsModule); ok {
		app.orgs = orgsMod
	}
	if permissionsMod, ok := mod.(PermissionsModule); ok {
		app.permissions = permissionsMod
	}
	if cacheMod, ok := mod.(CacheModule); ok {
		app.cache = cacheMod
	}
	if queueMod, ok := mod.(QueueModule); ok {
		app.queue = queueMod
	}
	if emailMod, ok := mod.(EmailModule); ok {
		app.email = emailMod
	}
	if eventsMod, ok := mod.(EventsModule); ok {
		app.events = eventsMod
	}

	return nil
}

// Shutdown gracefully stops all modules in reverse registration order.
func (app *App) Shutdown(ctx context.Context) error {
	app.mu.Lock()
	defer app.mu.Unlock()

	var errs []error
	for name, mod := range app.modules {
		if err := mod.Shutdown(ctx); err != nil {
			app.logger.Error("failed to shutdown module",
				"module", name,
				"error", err,
			)
			errs = append(errs, err)
		} else {
			app.logger.Info("module shutdown", "module", name)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("shutdown errors: %v", errs)
	}
	return nil
}

// Storage returns the storage module API.
// Panics if storage module is not registered.
func (app *App) Storage() StorageModule {
	if app.storage == nil {
		panic("storage module not registered")
	}
	return app.storage
}

// Users returns the users module API.
// Panics if users module is not registered.
func (app *App) Users() UsersModule {
	if app.users == nil {
		panic("users module not registered")
	}
	return app.users
}

// Auth returns the auth module API.
// Panics if auth module is not registered.
func (app *App) Auth() AuthModule {
	if app.auth == nil {
		panic("auth module not registered")
	}
	return app.auth
}

// Orgs returns the orgs module API.
// Panics if orgs module is not registered.
func (app *App) Orgs() OrgsModule {
	if app.orgs == nil {
		panic("orgs module not registered")
	}
	return app.orgs
}

// Permissions returns the permissions module API.
// Panics if permissions module is not registered.
func (app *App) Permissions() PermissionsModule {
	if app.permissions == nil {
		panic("permissions module not registered")
	}
	return app.permissions
}

// Cache returns the cache module API.
// Panics if cache module is not registered.
func (app *App) Cache() CacheModule {
	if app.cache == nil {
		panic("cache module not registered")
	}
	return app.cache
}

// Queue returns the queue module API.
// Panics if queue module is not registered.
func (app *App) Queue() QueueModule {
	if app.queue == nil {
		panic("queue module not registered")
	}
	return app.queue
}

// Email returns the email module API.
// Panics if email module is not registered.
func (app *App) Email() EmailModule {
	if app.email == nil {
		panic("email module not registered")
	}
	return app.email
}

// Events returns the events module API.
// Panics if events module is not registered.
func (app *App) Events() EventsModule {
	if app.events == nil {
		panic("events module not registered")
	}
	return app.events
}

// Logger returns the chassis logger for use by modules and application code.
func (app *App) Logger() *slog.Logger {
	return app.logger
}

// Config returns the chassis configuration.
func (app *App) Config() *Config {
	return app.config
}

// ConfigData returns the raw configuration data loaded from file.
// Returns nil if no config file was loaded.
func (app *App) ConfigData() ConfigData {
	return app.configData
}
