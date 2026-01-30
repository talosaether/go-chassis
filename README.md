# Go-Chassis

A modular Go foundation library providing common application infrastructure. Import what you need, configure with sensible defaults, swap implementations when needed.

## Philosophy

- **Import, don't copy** - Standard Go module dependency
- **Register what you need** - Opt-in modules, not a monolithic framework
- **Pluggable providers** - Interfaces with defaults, replaceable implementations
- **Explicit over magic** - No global state, everything flows through the App instance

## Quick Start

```bash
go get github.com/talosaether/chassis
```

```go
package main

import (
    "context"
    "log"

    "github.com/talosaether/chassis"
    "github.com/talosaether/chassis/auth"
    "github.com/talosaether/chassis/storage"
    "github.com/talosaether/chassis/users"
)

func main() {
    app := chassis.New(
        chassis.WithConfigFile("./config.yaml"),
        chassis.WithModules(
            storage.New(),
            users.New(),
            auth.New(),
        ),
    )
    defer app.Shutdown(context.Background())

    ctx := context.Background()

    // Create a user
    user, err := app.Users().Create(ctx, "user@example.com", "password123")
    if err != nil {
        log.Fatal(err)
    }

    // Store a file
    app.Storage().Put(ctx, "welcome.txt", []byte("Hello!"))

    // Authenticate
    authenticatedUser, err := app.Users().Authenticate(ctx, "user@example.com", "password123")
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Authenticated user: %v", authenticatedUser)
}
```

## Modules

### Foundation
| Module | Purpose | Default Provider |
|--------|---------|------------------|
| **storage** | File/blob storage | Local filesystem |
| **users** | User management & authentication | SQLite + Argon2id |
| **auth** | Session management | SQLite + cookies |

### Identity & Access
| Module | Purpose | Default Provider |
|--------|---------|------------------|
| **orgs** | Multi-tenancy / organizations | SQLite |
| **permissions** | Role-based access control | In-memory rules |

### Infrastructure
| Module | Purpose | Default Provider |
|--------|---------|------------------|
| **cache** | Key-value caching with TTL | In-memory |
| **queue** | Background job processing | SQLite |
| **email** | Transactional email | SMTP |
| **events** | Internal pub/sub | In-memory |

## Module Usage

### Storage

```go
app := chassis.New(chassis.WithModules(storage.New()))

ctx := context.Background()
app.Storage().Put(ctx, "files/doc.pdf", data)
data, err := app.Storage().Get(ctx, "files/doc.pdf")
files, err := app.Storage().List(ctx, "files/")
app.Storage().Delete(ctx, "files/doc.pdf")
```

### Users

```go
app := chassis.New(chassis.WithModules(users.New()))

ctx := context.Background()
user, err := app.Users().Create(ctx, "user@example.com", "password")
user, err := app.Users().GetByEmail(ctx, "user@example.com")
user, err := app.Users().Authenticate(ctx, "user@example.com", "password")
```

### Auth (Sessions)

```go
app := chassis.New(chassis.WithModules(
    users.New(),
    auth.New(auth.WithSessionTTL(48 * time.Hour)),
))

// In HTTP handler
func loginHandler(w http.ResponseWriter, r *http.Request) {
    session, err := app.Auth().(*auth.Module).Login(ctx, w, userID)
    // Session cookie is set automatically
}

// Protected routes
mux.Handle("/dashboard", app.Auth().(*auth.Module).RequireAuth(dashboardHandler))
```

### Organizations

```go
app := chassis.New(chassis.WithModules(
    orgs.New(),
    permissions.New(),
))

ctx := context.Background()

// Create org
org, _ := app.Orgs().Create(ctx, orgs.CreateInput{Name: "Acme Corp"})
orgID := org.(*orgs.Org).ID()

// Add members with roles
app.Orgs().AddMember(ctx, orgID, userID, "admin")

// Check permissions
if app.Permissions().Can(ctx, userID, "org:delete", orgID) {
    app.Orgs().Delete(ctx, orgID)
}
```

### Cache

```go
app := chassis.New(chassis.WithModules(
    cache.New(cache.WithDefaultTTL(10 * time.Minute)),
))

ctx := context.Background()
app.Cache().Set(ctx, "user:123", []byte(`{"name":"Alice"}`))
data, found := app.Cache().Get(ctx, "user:123")
```

### Queue

```go
app := chassis.New(chassis.WithModules(queue.New()))

ctx := context.Background()

// Enqueue a job
job, _ := app.Queue().Enqueue(ctx, "send-email", map[string]string{
    "to": "user@example.com",
})

// Process jobs with a worker
go app.Queue().(*queue.Module).Worker(ctx, func(ctx context.Context, job *queue.Job) error {
    // Process the job
    return nil
})
```

### Email

```go
app := chassis.New(chassis.WithModules(
    email.New(email.WithSMTPConfig(email.SMTPConfig{
        Host: "smtp.example.com",
        Port: 587,
        From: "noreply@example.com",
    })),
))

ctx := context.Background()
app.Email().Send(ctx, "user@example.com", "Welcome!", "Hello and welcome...")
```

### Events

```go
app := chassis.New(chassis.WithModules(events.New()))

// Subscribe
unsubscribe := app.Events().Subscribe("user.created", events.Handler(
    func(ctx context.Context, eventType string, payload any) {
        log.Printf("User created: %v", payload)
    },
))
defer unsubscribe()

// Publish
app.Events().Publish(ctx, "user.created", user)
```

## Configuration

### YAML Configuration

```yaml
# config.yaml
chassis:
  env: production
  log_level: info

storage:
  base_path: ./data/files

users:
  db_path: ./data/users.db

auth:
  db_path: ./data/sessions.db
  session_ttl: 24h
  cookie_name: session
  secure_cookie: true

orgs:
  db_path: ./data/orgs.db

cache:
  default_ttl: 5m

queue:
  db_path: ./data/queue.db

email:
  smtp_host: smtp.example.com
  smtp_port: 587
  smtp_username: ${SMTP_USER}
  smtp_password: ${SMTP_PASS}
  from: noreply@example.com
```

Environment variables are expanded using `${VAR}` or `${VAR:-default}` syntax.

### Programmatic Configuration

```go
app := chassis.New(
    chassis.WithConfig(&chassis.Config{
        Env:      "production",
        LogLevel: slog.LevelWarn,
    }),
    chassis.WithModules(
        storage.New(storage.WithBasePath("/var/data")),
        users.New(users.WithDBPath("/var/db/users.db")),
        auth.New(
            auth.WithSessionTTL(48 * time.Hour),
            auth.WithSecureCookie(true),
        ),
    ),
)
```

## Custom Providers

Each module defines interfaces for pluggable backends. See [docs/PROVIDERS.md](docs/PROVIDERS.md) for detailed implementation guides.

```go
// Implement the storage.Provider interface
type S3Provider struct {
    bucket string
    client *s3.Client
}

func (p *S3Provider) Put(ctx context.Context, key string, data []byte) error {
    // S3-specific implementation
}

func (p *S3Provider) Get(ctx context.Context, key string) ([]byte, error) {
    // S3-specific implementation
}

// ... implement Delete and List

// Use your custom provider
app := chassis.New(
    chassis.WithModules(
        storage.New(storage.WithProvider(&S3Provider{
            bucket: "my-bucket",
        })),
    ),
)
```

## Testing

Use provided test utilities and mock providers:

```go
// Use LogProvider for email testing
emailMod := email.New(email.WithProvider(
    email.NewLogProvider(func(to, subject, body string) {
        // Capture sent emails for assertions
    }),
))

// Use in-memory providers for fast tests
app := chassis.New(
    chassis.WithModules(
        cache.New(), // In-memory by default
        events.New(), // In-memory by default
    ),
)
```

## Project Structure

```
chassis/
├── chassis.go          # Core App type and lifecycle
├── config.go           # Configuration loading
├── module.go           # Module interface
├── auth/               # Authentication module
├── cache/              # Caching module
├── email/              # Email module
├── events/             # Pub/sub module
├── orgs/               # Organizations module
├── permissions/        # RBAC module
├── queue/              # Job queue module
├── storage/            # File storage module
├── users/              # User management module
├── cmd/demo/           # Example application
├── docs/               # Additional documentation
│   ├── PROVIDERS.md    # Custom provider guide
│   └── chassis-spec.md # Project specification
└── e2e/                # Integration tests
```

## Running Tests

```bash
# Run all tests
go test ./...

# Run with verbose output
go test -v ./...

# Run specific module tests
go test ./auth/...
go test ./users/...
```

## License

MIT
