# Implementing Custom Providers

This guide explains how to implement custom providers for chassis modules.

## Overview

Several chassis modules use the **provider pattern** to allow swapping implementations. For example:
- **storage**: Switch from local filesystem to S3, GCS, or R2
- **cache**: Switch from in-memory to Redis or Memcached
- **email**: Switch from SMTP to SendGrid, SES, or Mailgun

## Storage Provider

### Interface

```go
// Provider defines the interface for storage implementations.
type Provider interface {
    Put(ctx context.Context, key string, data []byte) error
    Get(ctx context.Context, key string) ([]byte, error)
    Delete(ctx context.Context, key string) error
    List(ctx context.Context, prefix string) ([]string, error)
}
```

### Example: S3 Provider

```go
package s3storage

import (
    "bytes"
    "context"
    "io"

    "github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3Provider struct {
    client *s3.Client
    bucket string
}

func New(client *s3.Client, bucket string) *S3Provider {
    return &S3Provider{client: client, bucket: bucket}
}

func (p *S3Provider) Put(ctx context.Context, key string, data []byte) error {
    _, err := p.client.PutObject(ctx, &s3.PutObjectInput{
        Bucket: &p.bucket,
        Key:    &key,
        Body:   bytes.NewReader(data),
    })
    return err
}

func (p *S3Provider) Get(ctx context.Context, key string) ([]byte, error) {
    result, err := p.client.GetObject(ctx, &s3.GetObjectInput{
        Bucket: &p.bucket,
        Key:    &key,
    })
    if err != nil {
        return nil, err
    }
    defer result.Body.Close()
    return io.ReadAll(result.Body)
}

func (p *S3Provider) Delete(ctx context.Context, key string) error {
    _, err := p.client.DeleteObject(ctx, &s3.DeleteObjectInput{
        Bucket: &p.bucket,
        Key:    &key,
    })
    return err
}

func (p *S3Provider) List(ctx context.Context, prefix string) ([]string, error) {
    result, err := p.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
        Bucket: &p.bucket,
        Prefix: &prefix,
    })
    if err != nil {
        return nil, err
    }

    keys := make([]string, len(result.Contents))
    for i, obj := range result.Contents {
        keys[i] = *obj.Key
    }
    return keys, nil
}
```

### Usage

```go
import (
    "github.com/aws/aws-sdk-go-v2/config"
    "github.com/aws/aws-sdk-go-v2/service/s3"
    "github.com/talosaether/chassis"
    "github.com/talosaether/chassis/storage"
    "your/project/s3storage"
)

func main() {
    cfg, _ := config.LoadDefaultConfig(context.Background())
    s3Client := s3.NewFromConfig(cfg)

    app := chassis.New(
        chassis.WithModules(
            storage.New(storage.WithProvider(
                s3storage.New(s3Client, "my-bucket"),
            )),
        ),
    )
}
```

## Cache Provider

### Interface

```go
// Provider defines the interface for cache implementations.
type Provider interface {
    Get(ctx context.Context, key string) ([]byte, bool)
    Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
    Delete(ctx context.Context, key string) error
    Clear(ctx context.Context) error
}
```

### Example: Redis Provider

```go
package rediscache

import (
    "context"
    "time"

    "github.com/redis/go-redis/v9"
)

type RedisProvider struct {
    client *redis.Client
}

func New(addr string) *RedisProvider {
    return &RedisProvider{
        client: redis.NewClient(&redis.Options{Addr: addr}),
    }
}

func (p *RedisProvider) Get(ctx context.Context, key string) ([]byte, bool) {
    val, err := p.client.Get(ctx, key).Bytes()
    if err != nil {
        return nil, false
    }
    return val, true
}

func (p *RedisProvider) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
    return p.client.Set(ctx, key, value, ttl).Err()
}

func (p *RedisProvider) Delete(ctx context.Context, key string) error {
    return p.client.Del(ctx, key).Err()
}

func (p *RedisProvider) Clear(ctx context.Context) error {
    return p.client.FlushDB(ctx).Err()
}
```

### Usage

```go
import (
    "github.com/talosaether/chassis"
    "github.com/talosaether/chassis/cache"
    "your/project/rediscache"
)

func main() {
    app := chassis.New(
        chassis.WithModules(
            cache.New(cache.WithProvider(
                rediscache.New("localhost:6379"),
            )),
        ),
    )
}
```

## Email Provider

### Interface

```go
// Provider defines the interface for email sending implementations.
type Provider interface {
    Send(ctx context.Context, to, subject, body string) error
}

// HTMLProvider is an optional interface for providers that support HTML emails.
type HTMLProvider interface {
    Provider
    SendHTML(ctx context.Context, to, subject, htmlBody string) error
}
```

### Example: SendGrid Provider

```go
package sendgridemail

import (
    "context"

    "github.com/sendgrid/sendgrid-go"
    "github.com/sendgrid/sendgrid-go/helpers/mail"
)

type SendGridProvider struct {
    client *sendgrid.Client
    from   *mail.Email
}

func New(apiKey, fromEmail, fromName string) *SendGridProvider {
    return &SendGridProvider{
        client: sendgrid.NewSendClient(apiKey),
        from:   mail.NewEmail(fromName, fromEmail),
    }
}

func (p *SendGridProvider) Send(ctx context.Context, to, subject, body string) error {
    toEmail := mail.NewEmail("", to)
    message := mail.NewSingleEmail(p.from, subject, toEmail, body, "")
    _, err := p.client.Send(message)
    return err
}

func (p *SendGridProvider) SendHTML(ctx context.Context, to, subject, htmlBody string) error {
    toEmail := mail.NewEmail("", to)
    message := mail.NewSingleEmail(p.from, subject, toEmail, "", htmlBody)
    _, err := p.client.Send(message)
    return err
}
```

### Usage

```go
import (
    "os"
    "github.com/talosaether/chassis"
    "github.com/talosaether/chassis/email"
    "your/project/sendgridemail"
)

func main() {
    app := chassis.New(
        chassis.WithModules(
            email.New(email.WithProvider(
                sendgridemail.New(
                    os.Getenv("SENDGRID_API_KEY"),
                    "noreply@example.com",
                    "My App",
                ),
            )),
        ),
    )
}
```

## Custom Store (Users, Auth, Orgs, Queue)

Modules with persistent state (users, auth, orgs, queue) use **stores** rather than providers. The pattern is similar.

### Users Store Interface

```go
type Store interface {
    Create(ctx context.Context, user *User) error
    GetByID(ctx context.Context, id string) (*User, error)
    GetByEmail(ctx context.Context, email string) (*User, error)
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, id string) error
    Close() error
}
```

### Example: PostgreSQL Users Store

```go
package pgusers

import (
    "context"
    "database/sql"

    "github.com/talosaether/chassis/users"
    _ "github.com/lib/pq"
)

type PostgresStore struct {
    db *sql.DB
}

func New(connStr string) (*PostgresStore, error) {
    db, err := sql.Open("postgres", connStr)
    if err != nil {
        return nil, err
    }

    // Run migrations
    _, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS users (
            id TEXT PRIMARY KEY,
            email TEXT UNIQUE NOT NULL,
            password_hash TEXT NOT NULL,
            created_at TIMESTAMP NOT NULL,
            updated_at TIMESTAMP NOT NULL
        )
    `)
    if err != nil {
        return nil, err
    }

    return &PostgresStore{db: db}, nil
}

func (s *PostgresStore) Create(ctx context.Context, user *users.User) error {
    _, err := s.db.ExecContext(ctx,
        `INSERT INTO users (id, email, password_hash, created_at, updated_at)
         VALUES ($1, $2, $3, $4, $5)`,
        user.GetID(), user.GetEmail(), user.PasswordHash,
        user.CreatedAt, user.UpdatedAt,
    )
    return err
}

// ... implement remaining methods

func (s *PostgresStore) Close() error {
    return s.db.Close()
}
```

### Usage

```go
import (
    "github.com/talosaether/chassis"
    "github.com/talosaether/chassis/users"
    "your/project/pgusers"
)

func main() {
    pgStore, _ := pgusers.New("postgres://localhost/myapp")

    app := chassis.New(
        chassis.WithModules(
            users.New(users.WithStore(pgStore)),
        ),
    )
}
```

## Testing Custom Providers

Use interfaces for easy mocking in tests:

```go
type mockStorageProvider struct {
    data map[string][]byte
}

func (m *mockStorageProvider) Put(ctx context.Context, key string, data []byte) error {
    m.data[key] = data
    return nil
}

func (m *mockStorageProvider) Get(ctx context.Context, key string) ([]byte, error) {
    if data, ok := m.data[key]; ok {
        return data, nil
    }
    return nil, errors.New("not found")
}

// ... implement remaining methods

func TestMyFeature(t *testing.T) {
    mock := &mockStorageProvider{data: make(map[string][]byte)}

    app := chassis.New(
        chassis.WithModules(
            storage.New(storage.WithProvider(mock)),
        ),
    )

    // Test your feature
}
```

## Best Practices

1. **Accept context**: All methods should accept `context.Context` for cancellation and timeouts.

2. **Return errors**: Don't panic. Return errors and let the application decide how to handle them.

3. **Thread safety**: Providers may be called from multiple goroutines. Use proper synchronization.

4. **Resource cleanup**: Implement `Close()` if your provider holds connections or other resources.

5. **Configuration**: Accept configuration via constructor parameters, not global variables.

6. **Logging**: Use the chassis logger through the App instance rather than a separate logger.

```go
func (p *MyProvider) DoSomething(ctx context.Context) error {
    // Access logger via app reference if needed
    p.app.Logger().Info("doing something")
    return nil
}
```
