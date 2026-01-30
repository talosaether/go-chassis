# Crucible Chassis: A Go Foundation Library

## Philosophy

Build a reusable Go module that provides common application infrastructure, designed for:

1. **Import, don't copy** - Standard Go module dependency, not boilerplate generation
2. **Register what you need** - Opt-in modules, not a monolithic framework
3. **Pluggable providers** - Interfaces with sensible defaults, replaceable implementations
4. **Agent-friendly** - Clean public APIs with clear documentation for AI-assisted development
5. **Dogfood-driven** - Features emerge from real usage across multiple projects

---

## Core Concepts

### Module System

A module is a self-contained capability (Auth, Storage, Queue, etc.) that:
- Implements a standard `Module` interface
- Can be registered with the chassis at startup
- Exposes a clean public API
- Accepts configuration and optional custom providers

```go
// The universal module contract
type Module interface {
    Name() string
    Init(config Config) error
    Shutdown() error
}
```

### Provider Pattern

Each module defines interfaces for its pluggable components:

```go
// Storage module defines what a storage provider must do
type StorageProvider interface {
    Put(ctx context.Context, key string, data []byte) error
    Get(ctx context.Context, key string) ([]byte, error)
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, prefix string) ([]string, error)
}

// Built-in: LocalFS, S3, GCS
// Custom: Implement the interface, pass it in
```

### Configuration

Hierarchical config with environment override:

```yaml
# config.yaml
chassis:
  env: development
  
auth:
  session_ttl: 24h
  jwt_secret: ${AUTH_JWT_SECRET}
  
storage:
  provider: local
  local:
    base_path: ./data
```

---

## Module Inventory

### Phase 1: Foundation
| Module | Purpose | Default Provider |
|--------|---------|------------------|
| **Config** | Configuration loading & access | YAML + env vars |
| **Logger** | Structured logging | slog (stdlib) |
| **Storage** | File/blob storage | Local filesystem |

### Phase 2: Identity
| Module | Purpose | Default Provider |
|--------|---------|------------------|
| **Auth** | Authentication (sessions, JWT) | Cookie sessions |
| **Users** | User management & profiles | SQLite |
| **Orgs** | Multi-tenancy / organizations | SQLite |
| **Permissions** | RBAC / access control | In-memory rules |

### Phase 3: Infrastructure
| Module | Purpose | Default Provider |
|--------|---------|------------------|
| **Cache** | Key-value caching | In-memory (bigcache) |
| **Queue** | Background job processing | SQLite-backed |
| **Email** | Transactional email | SMTP |
| **Events** | Internal pub/sub | In-memory bus |

### Phase 4: Application
| Module | Purpose | Default Provider |
|--------|---------|------------------|
| **Content** | CMS-style content storage | Markdown + SQLite |
| **Captcha** | Bot protection | hCaptcha |
| **Uploads** | File upload handling | Local + Storage module |

---

## Example Usage

### Child Project: A Simple SaaS App

```go
package main

import (
    "github.com/bottah/chassis"
    "github.com/bottah/chassis/auth"
    "github.com/bottah/chassis/storage"
    "github.com/bottah/chassis/users"
)

func main() {
    // Initialize chassis with selected modules
    app := chassis.New(
        chassis.WithConfig("./config.yaml"),
        chassis.WithModules(
            storage.New(),           // Use defaults
            users.New(),             // Use defaults  
            auth.New(auth.Options{   // Customize
                SessionTTL: 48 * time.Hour,
            }),
        ),
    )
    
    // Access modules through the app
    app.Storage().Put(ctx, "files/doc.pdf", data)
    
    user, err := app.Users().Create(ctx, users.CreateInput{
        Email: "test@example.com",
    })
    
    // Start HTTP server with built-in middleware
    app.Serve(":8080", myRoutes)
}
```

### Custom Provider Example

```go
// Your project needs R2 storage instead of S3
type R2Provider struct {
    accountID string
    bucket    string
    client    *r2.Client
}

func (r *R2Provider) Put(ctx context.Context, key string, data []byte) error {
    // R2-specific implementation
}

func (r *R2Provider) Get(ctx context.Context, key string) ([]byte, error) {
    // R2-specific implementation
}

// ... implement full StorageProvider interface

func main() {
    app := chassis.New(
        chassis.WithModules(
            storage.New(storage.WithProvider(&R2Provider{
                accountID: os.Getenv("R2_ACCOUNT"),
                bucket:    "my-bucket",
            })),
        ),
    )
}
```

---

## Project Structure

```
chassis/
├── go.mod                    # github.com/bottah/chassis
├── chassis.go                # Core App type and lifecycle
├── config.go                 # Configuration loading
├── module.go                 # Module interface definitions
│
├── auth/
│   ├── auth.go               # Public API
│   ├── module.go             # Module implementation
│   ├── providers.go          # Provider interfaces
│   ├── session_provider.go   # Default session implementation
│   └── jwt_provider.go       # JWT implementation
│
├── storage/
│   ├── storage.go            # Public API
│   ├── module.go             # Module implementation
│   ├── providers.go          # Provider interface
│   ├── local_provider.go     # Filesystem implementation
│   └── s3_provider.go        # S3 implementation
│
├── users/
│   ├── users.go              # Public API
│   ├── module.go             # Module implementation
│   ├── models.go             # User struct, CreateInput, etc.
│   └── sqlite_store.go       # Default SQLite storage
│
└── internal/
    └── registry/             # Module registry internals
```

---

## Design Principles

### 1. Explicit Over Magic
No global state, no init() side effects. Everything flows through the App instance.

```go
// Bad: magic globals
auth.CurrentUser(r)

// Good: explicit context
app.Auth().UserFromRequest(r)
```

### 2. Interfaces at Boundaries
Public module APIs return interfaces, not concrete types:

```go
// users/users.go
type User interface {
    ID() string
    Email() string
    CreatedAt() time.Time
}

// Internal implementation is private
type user struct { ... }
```

### 3. Sensible Defaults, Full Override
Every module works out of the box with zero config, but everything is replaceable:

```go
// Zero config - works with SQLite, local storage, etc.
app := chassis.New()

// Full control
app := chassis.New(
    chassis.WithModules(
        users.New(
            users.WithStore(myPostgresStore),
            users.WithValidator(myCustomValidator),
        ),
    ),
)
```

### 4. Errors Are Values
No panics in library code. Return errors, let the application decide:

```go
user, err := app.Users().ByEmail(ctx, email)
if errors.Is(err, users.ErrNotFound) {
    // Handle missing user
}
```

### 5. Context Everywhere
All operations accept context for cancellation, timeouts, and request-scoped values:

```go
func (m *Module) DoThing(ctx context.Context, input Input) (Output, error)
```

---

## Implementation Roadmap

### Milestone 1: Walking Skeleton (Week 1-2)
**Goal**: Import chassis into a child project, register a module, call it

- [ ] Basic `chassis.New()` and `App` type
- [ ] Module interface and registry
- [ ] Config loading (YAML + env)
- [ ] Logger module (wrapping slog)
- [ ] Storage module with local filesystem provider
- [ ] Example child project that compiles and runs

**Definition of Done**: 
```go
app := chassis.New()
app.Storage().Put(ctx, "test.txt", []byte("hello"))
data, _ := app.Storage().Get(ctx, "test.txt")
// data == []byte("hello")
```

### Milestone 2: Identity Foundation (Week 3-4)
**Goal**: Register, login, manage users

- [ ] Users module with SQLite store
- [ ] Auth module with session provider
- [ ] Password hashing (argon2)
- [ ] HTTP middleware for auth
- [ ] Basic CRUD for users

**Definition of Done**:
```go
user, _ := app.Users().Create(ctx, users.CreateInput{
    Email: "test@example.com",
    Password: "secret",
})
session, _ := app.Auth().Login(ctx, "test@example.com", "secret")
// session.UserID == user.ID()
```

### Milestone 3: Organizations & Permissions (Week 5-6)
**Goal**: Multi-tenancy support

- [ ] Orgs module
- [ ] User-Org membership
- [ ] Permissions module with role-based access
- [ ] Middleware for org-scoped requests

**Definition of Done**:
```go
org, _ := app.Orgs().Create(ctx, orgs.CreateInput{Name: "Acme"})
app.Orgs().AddMember(ctx, org.ID(), user.ID(), "admin")
can := app.Permissions().Can(ctx, user.ID(), "org:delete", org.ID())
// can == true for admin
```

### Milestone 4: Infrastructure Modules (Week 7-8)
**Goal**: Production-ready supporting infrastructure

- [ ] Cache module (in-memory default, Redis provider)
- [ ] Queue module (SQLite default)
- [ ] Email module (SMTP default)
- [ ] Events module for internal pub/sub

### Milestone 5: Documentation & Polish (Week 9-10)
**Goal**: Agent-friendly, ready for real use

- [ ] Complete godoc for all public APIs
- [ ] README with quick start
- [ ] Example projects (API server, web app)
- [ ] Provider implementation guide
- [ ] Integration tests

---

## Learning Go Along the Way

This project will teach you:

1. **Go modules** - Publishing, versioning, dependency management
2. **Interfaces** - The core abstraction mechanism in Go
3. **Error handling** - Explicit, value-based error patterns
4. **Context** - Request-scoped data, cancellation, timeouts
5. **Testing** - Table-driven tests, mocks via interfaces
6. **Project structure** - Idiomatic package organization
7. **Concurrency** - Goroutines for queue workers, connection pools

Start small. Get Milestone 1 working. The rest will follow.
