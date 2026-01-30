# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build and Run Commands

```bash
# Run the demo application
go run ./cmd/demo

# Build the module
go build ./...

# Run tests
go test ./...

# Run a single test
go test -run TestName ./...
```

## Architecture

Go-chassis is a reusable Go module providing common application infrastructure with an opt-in, pluggable module system.

### Core Concepts

**App (`chassis.go`)**: Central instance holding registered modules, configuration, and logger. Created via `chassis.New()` with functional options.

**Module Interface (`module.go`)**: All modules implement `Name()`, `Init(ctx, *App)`, and `Shutdown(ctx)`. Modules receive the App during init for accessing logger and config.

**Provider Pattern**: Modules define interfaces for pluggable backends. Default implementations are provided but can be replaced. Example: `storage.Provider` interface with `LocalProvider` default.

### Module Registration Flow

1. `chassis.New()` creates App with defaults
2. `WithModules()` option calls `App.Register()` for each module
3. `Register()` calls `module.Init()` and stores in registry
4. Typed accessors (e.g., `app.Storage()`) are wired up via type assertion

### Configuration (`config.go`)

YAML-based config with environment variable expansion:
- Load via `WithConfigFile("./config.yaml")`
- Access values via `app.ConfigData().GetString("storage.base_path")`
- Env var syntax: `${VAR}` or `${VAR:-default}`
- Modules can read their config section during `Init()`

### Project Structure

```
chassis.go, module.go    # Core App and Module interface
config.go                # YAML config loading with env var expansion
storage/storage.go       # Storage module with Provider pattern
users/                   # Users module (users.go, store.go)
auth/                    # Auth module (auth.go, store.go)
cmd/demo/main.go         # Example HTTP server with auth
config.yaml              # Example configuration
```

### Current Modules

- **Storage** (`storage/`): File/blob storage with `Provider` interface. Default: local filesystem. Operations: Put, Get, Delete, List.

- **Users** (`users/`): User management with SQLite store. Features: Create, GetByID, GetByEmail, Update, Delete, Authenticate. Password hashing with Argon2id.

- **Auth** (`auth/`): Session-based authentication. Features: Login, Logout, GetSession, RequireAuth middleware. Cookie-based sessions with SQLite store. Configurable TTL and secure cookies.

### Design Patterns

- No global state or `init()` side effects - everything flows through App instance
- All operations accept `context.Context` as first parameter
- Errors are values, not panics (except module accessor when module not registered)
- Configuration via functional options: `WithConfigFile()`, `WithModules()`, `WithProvider()`, `WithBasePath()`
