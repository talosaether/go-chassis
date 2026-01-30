package email

import (
	"context"
	"testing"
)

func TestModule_Name(t *testing.T) {
	mod := New()
	if mod.Name() != "email" {
		t.Errorf("Name() should return 'email', got %q", mod.Name())
	}
}

func TestModuleNew_DefaultOptions(t *testing.T) {
	mod := New()

	if mod.smtpConfig.Host != "localhost" {
		t.Errorf("default host should be 'localhost', got %q", mod.smtpConfig.Host)
	}
	if mod.smtpConfig.Port != 25 {
		t.Errorf("default port should be 25, got %d", mod.smtpConfig.Port)
	}
}

func TestModuleNew_WithSMTPConfig(t *testing.T) {
	config := SMTPConfig{
		Host:     "smtp.example.com",
		Port:     587,
		Username: "user@example.com",
		Password: "secret",
		From:     "noreply@example.com",
	}

	mod := New(WithSMTPConfig(config))

	if mod.smtpConfig.Host != config.Host {
		t.Errorf("host should be %q, got %q", config.Host, mod.smtpConfig.Host)
	}
	if mod.smtpConfig.Port != config.Port {
		t.Errorf("port should be %d, got %d", config.Port, mod.smtpConfig.Port)
	}
	if mod.smtpConfig.Username != config.Username {
		t.Errorf("username should be %q, got %q", config.Username, mod.smtpConfig.Username)
	}
	if mod.smtpConfig.From != config.From {
		t.Errorf("from should be %q, got %q", config.From, mod.smtpConfig.From)
	}
}

func TestModuleNew_WithProvider(t *testing.T) {
	provider := NewLogProvider(nil)
	mod := New(WithProvider(provider))

	if mod.provider != provider {
		t.Error("custom provider should be set")
	}
}

// LogProvider tests

func TestLogProvider_Send(t *testing.T) {
	var logged struct {
		to      string
		subject string
		body    string
	}

	provider := NewLogProvider(func(to, subject, body string) {
		logged.to = to
		logged.subject = subject
		logged.body = body
	})

	ctx := context.Background()
	err := provider.Send(ctx, "test@example.com", "Test Subject", "Test Body")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if logged.to != "test@example.com" {
		t.Errorf("to should be 'test@example.com', got %q", logged.to)
	}
	if logged.subject != "Test Subject" {
		t.Errorf("subject should be 'Test Subject', got %q", logged.subject)
	}
	if logged.body != "Test Body" {
		t.Errorf("body should be 'Test Body', got %q", logged.body)
	}
}

func TestLogProvider_SendHTML(t *testing.T) {
	var logged struct {
		to      string
		subject string
		body    string
	}

	provider := NewLogProvider(func(to, subject, body string) {
		logged.to = to
		logged.subject = subject
		logged.body = body
	})

	ctx := context.Background()
	htmlBody := "<html><body><h1>Hello</h1></body></html>"
	err := provider.SendHTML(ctx, "test@example.com", "HTML Subject", htmlBody)
	if err != nil {
		t.Fatalf("SendHTML failed: %v", err)
	}

	if logged.body != htmlBody {
		t.Errorf("body should be %q, got %q", htmlBody, logged.body)
	}
}

func TestLogProvider_SendWithNilLogger(t *testing.T) {
	provider := NewLogProvider(nil)
	ctx := context.Background()

	// Should not panic with nil logger
	err := provider.Send(ctx, "test@example.com", "Subject", "Body")
	if err != nil {
		t.Errorf("Send should not error with nil logger, got: %v", err)
	}
}

// SMTPProvider tests (configuration only, no actual sending)

func TestSMTPProvider_New(t *testing.T) {
	config := SMTPConfig{
		Host:     "smtp.example.com",
		Port:     587,
		Username: "user",
		Password: "pass",
		From:     "from@example.com",
	}

	provider := NewSMTPProvider(config)

	if provider.config.Host != config.Host {
		t.Errorf("host mismatch: got %q, want %q", provider.config.Host, config.Host)
	}
	if provider.config.Port != config.Port {
		t.Errorf("port mismatch: got %d, want %d", provider.config.Port, config.Port)
	}
}

// Module with LogProvider integration test

func TestModule_SendWithLogProvider(t *testing.T) {
	var sent bool

	provider := NewLogProvider(func(to, subject, body string) {
		sent = true
	})

	mod := New(WithProvider(provider))
	ctx := context.Background()

	err := mod.Send(ctx, "recipient@example.com", "Subject", "Body")
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if !sent {
		t.Error("email should have been 'sent' (logged)")
	}
}

func TestModule_SendHTMLWithLogProvider(t *testing.T) {
	var sent bool

	provider := NewLogProvider(func(to, subject, body string) {
		sent = true
	})

	mod := New(WithProvider(provider))
	ctx := context.Background()

	err := mod.SendHTML(ctx, "recipient@example.com", "Subject", "<html>body</html>")
	if err != nil {
		t.Fatalf("SendHTML failed: %v", err)
	}

	if !sent {
		t.Error("HTML email should have been 'sent' (logged)")
	}
}

func TestModule_Shutdown(t *testing.T) {
	mod := New()
	ctx := context.Background()

	err := mod.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown should not error, got: %v", err)
	}
}

// SMTPConfig tests

func TestSMTPConfig_Defaults(t *testing.T) {
	config := SMTPConfig{}

	// Zero values
	if config.Host != "" {
		t.Errorf("default host should be empty, got %q", config.Host)
	}
	if config.Port != 0 {
		t.Errorf("default port should be 0, got %d", config.Port)
	}
}
