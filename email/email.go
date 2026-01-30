// Package email provides transactional email sending for the chassis framework.
//
// It supports both plain text and HTML emails via pluggable providers.
// The default provider sends email via SMTP.
//
// # Usage
//
// Register the module with chassis:
//
//	app := chassis.New(
//	    chassis.WithModules(
//	        email.New(),
//	    ),
//	)
//
// Send emails:
//
//	// Plain text
//	err := app.Email().Send(ctx, "user@example.com", "Welcome!", "Hello and welcome...")
//
//	// HTML
//	err := app.Email().SendHTML(ctx, "user@example.com", "Welcome!", "<h1>Hello!</h1>")
//
// # Configuration
//
// Configure via config.yaml:
//
//	email:
//	  smtp_host: smtp.example.com
//	  smtp_port: 587
//	  smtp_username: ${SMTP_USER}
//	  smtp_password: ${SMTP_PASS}
//	  from: noreply@example.com
//
// Or programmatically:
//
//	email.New(email.WithSMTPConfig(email.SMTPConfig{
//	    Host:     "smtp.example.com",
//	    Port:     587,
//	    Username: "user",
//	    Password: "pass",
//	    From:     "noreply@example.com",
//	}))
//
// # Testing
//
// Use LogProvider for development/testing:
//
//	email.New(email.WithProvider(email.NewLogProvider(func(to, subject, body string) {
//	    log.Printf("Email to %s: %s", to, subject)
//	})))
package email

import (
	"context"
	"fmt"
	"net/smtp"

	"github.com/talosaether/chassis"
)

// Provider defines the interface for email sending implementations.
type Provider interface {
	Send(ctx context.Context, to, subject, body string) error
}

// SMTPConfig holds SMTP server configuration.
type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
}

// Module is the email module implementation.
type Module struct {
	provider   Provider
	smtpConfig SMTPConfig
	app        *chassis.App
}

// Option is a function that configures the email module.
type Option func(*Module)

// WithProvider sets a custom email provider.
func WithProvider(provider Provider) Option {
	return func(mod *Module) {
		mod.provider = provider
	}
}

// WithSMTPConfig sets the SMTP configuration.
func WithSMTPConfig(config SMTPConfig) Option {
	return func(mod *Module) {
		mod.smtpConfig = config
	}
}

// New creates a new email module with the given options.
func New(opts ...Option) *Module {
	mod := &Module{
		smtpConfig: SMTPConfig{
			Host: "localhost",
			Port: 25,
		},
	}

	for _, opt := range opts {
		opt(mod)
	}

	return mod
}

// Name returns the module identifier.
func (mod *Module) Name() string {
	return "email"
}

// Init initializes the email module.
func (mod *Module) Init(ctx context.Context, app *chassis.App) error {
	mod.app = app

	// Read config if available
	if cfg := app.ConfigData(); cfg != nil {
		if host := cfg.GetString("email.smtp_host"); host != "" {
			mod.smtpConfig.Host = host
		}
		if port := cfg.GetInt("email.smtp_port"); port > 0 {
			mod.smtpConfig.Port = port
		}
		if username := cfg.GetString("email.smtp_username"); username != "" {
			mod.smtpConfig.Username = username
		}
		if password := cfg.GetString("email.smtp_password"); password != "" {
			mod.smtpConfig.Password = password
		}
		if from := cfg.GetString("email.from"); from != "" {
			mod.smtpConfig.From = from
		}
	}

	// Use default SMTP provider if none provided
	if mod.provider == nil {
		mod.provider = NewSMTPProvider(mod.smtpConfig)
	}

	app.Logger().Info("email module initialized", "smtp_host", mod.smtpConfig.Host, "smtp_port", mod.smtpConfig.Port)
	return nil
}

// Shutdown cleans up the email module.
func (mod *Module) Shutdown(ctx context.Context) error {
	return nil
}

// Send sends an email using the configured provider.
func (mod *Module) Send(ctx context.Context, to, subject, body string) error {
	return mod.provider.Send(ctx, to, subject, body)
}

// SendHTML sends an HTML email.
func (mod *Module) SendHTML(ctx context.Context, to, subject, htmlBody string) error {
	if htmlProvider, ok := mod.provider.(HTMLProvider); ok {
		return htmlProvider.SendHTML(ctx, to, subject, htmlBody)
	}
	// Fall back to plain text
	return mod.provider.Send(ctx, to, subject, htmlBody)
}

// HTMLProvider is an optional interface for providers that support HTML emails.
type HTMLProvider interface {
	Provider
	SendHTML(ctx context.Context, to, subject, htmlBody string) error
}

// SMTPProvider sends emails via SMTP.
type SMTPProvider struct {
	config SMTPConfig
}

// NewSMTPProvider creates a new SMTP email provider.
func NewSMTPProvider(config SMTPConfig) *SMTPProvider {
	return &SMTPProvider{config: config}
}

func (provider *SMTPProvider) Send(ctx context.Context, to, subject, body string) error {
	from := provider.config.From
	if from == "" {
		from = provider.config.Username
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\n\r\n%s",
		from, to, subject, body)

	addr := fmt.Sprintf("%s:%d", provider.config.Host, provider.config.Port)

	var auth smtp.Auth
	if provider.config.Username != "" {
		auth = smtp.PlainAuth("", provider.config.Username, provider.config.Password, provider.config.Host)
	}

	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

func (provider *SMTPProvider) SendHTML(ctx context.Context, to, subject, htmlBody string) error {
	from := provider.config.From
	if from == "" {
		from = provider.config.Username
	}

	msg := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=\"UTF-8\"\r\n\r\n%s",
		from, to, subject, htmlBody)

	addr := fmt.Sprintf("%s:%d", provider.config.Host, provider.config.Port)

	var auth smtp.Auth
	if provider.config.Username != "" {
		auth = smtp.PlainAuth("", provider.config.Username, provider.config.Password, provider.config.Host)
	}

	return smtp.SendMail(addr, auth, from, []string{to}, []byte(msg))
}

// LogProvider is a provider that logs emails instead of sending them.
// Useful for development and testing.
type LogProvider struct {
	logger func(to, subject, body string)
}

// NewLogProvider creates a provider that logs emails.
func NewLogProvider(logger func(to, subject, body string)) *LogProvider {
	return &LogProvider{logger: logger}
}

func (provider *LogProvider) Send(ctx context.Context, to, subject, body string) error {
	if provider.logger != nil {
		provider.logger(to, subject, body)
	}
	return nil
}

func (provider *LogProvider) SendHTML(ctx context.Context, to, subject, htmlBody string) error {
	return provider.Send(ctx, to, subject, htmlBody)
}
