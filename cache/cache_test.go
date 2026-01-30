package cache

import (
	"context"
	"testing"
	"time"
)

func TestMemoryProvider_SetAndGet(t *testing.T) {
	provider := NewMemoryProvider()
	ctx := context.Background()

	// Test Set and Get
	key := "test-key"
	value := []byte("test-value")
	ttl := 5 * time.Minute

	err := provider.Set(ctx, key, value, ttl)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	got, found := provider.Get(ctx, key)
	if !found {
		t.Fatal("Get returned not found for existing key")
	}
	if string(got) != string(value) {
		t.Errorf("Get returned %q, want %q", got, value)
	}
}

func TestMemoryProvider_GetNotFound(t *testing.T) {
	provider := NewMemoryProvider()
	ctx := context.Background()

	_, found := provider.Get(ctx, "nonexistent-key")
	if found {
		t.Error("Get returned found for nonexistent key")
	}
}

func TestMemoryProvider_Delete(t *testing.T) {
	provider := NewMemoryProvider()
	ctx := context.Background()

	key := "to-delete"
	value := []byte("delete-me")

	provider.Set(ctx, key, value, time.Hour)

	// Verify it exists
	_, found := provider.Get(ctx, key)
	if !found {
		t.Fatal("key should exist before delete")
	}

	// Delete it
	err := provider.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	_, found = provider.Get(ctx, key)
	if found {
		t.Error("key should not exist after delete")
	}
}

func TestMemoryProvider_DeleteNonexistent(t *testing.T) {
	provider := NewMemoryProvider()
	ctx := context.Background()

	// Deleting nonexistent key should not error
	err := provider.Delete(ctx, "nonexistent")
	if err != nil {
		t.Errorf("Delete of nonexistent key should not error, got: %v", err)
	}
}

func TestMemoryProvider_Clear(t *testing.T) {
	provider := NewMemoryProvider()
	ctx := context.Background()

	// Add multiple items
	provider.Set(ctx, "key1", []byte("value1"), time.Hour)
	provider.Set(ctx, "key2", []byte("value2"), time.Hour)
	provider.Set(ctx, "key3", []byte("value3"), time.Hour)

	// Clear all
	err := provider.Clear(ctx)
	if err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	// Verify all are gone
	for _, key := range []string{"key1", "key2", "key3"} {
		if _, found := provider.Get(ctx, key); found {
			t.Errorf("key %q should not exist after Clear", key)
		}
	}
}

func TestMemoryProvider_TTLExpiration(t *testing.T) {
	provider := NewMemoryProvider()
	ctx := context.Background()

	key := "short-lived"
	value := []byte("expires-soon")

	// Set with very short TTL
	err := provider.Set(ctx, key, value, 10*time.Millisecond)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	// Should exist immediately
	_, found := provider.Get(ctx, key)
	if !found {
		t.Fatal("key should exist immediately after set")
	}

	// Wait for expiration
	time.Sleep(20 * time.Millisecond)

	// Should be expired now
	_, found = provider.Get(ctx, key)
	if found {
		t.Error("key should be expired after TTL")
	}
}

func TestMemoryProvider_Overwrite(t *testing.T) {
	provider := NewMemoryProvider()
	ctx := context.Background()

	key := "overwrite-key"
	value1 := []byte("first-value")
	value2 := []byte("second-value")

	provider.Set(ctx, key, value1, time.Hour)

	got, _ := provider.Get(ctx, key)
	if string(got) != string(value1) {
		t.Errorf("first value should be %q, got %q", value1, got)
	}

	// Overwrite
	provider.Set(ctx, key, value2, time.Hour)

	got, _ = provider.Get(ctx, key)
	if string(got) != string(value2) {
		t.Errorf("after overwrite, value should be %q, got %q", value2, got)
	}
}

func TestMemoryProvider_Concurrent(t *testing.T) {
	provider := NewMemoryProvider()
	ctx := context.Background()

	// Run concurrent reads and writes
	done := make(chan bool)

	// Writer goroutine
	go func() {
		for i := 0; i < 100; i++ {
			provider.Set(ctx, "concurrent-key", []byte("value"), time.Hour)
		}
		done <- true
	}()

	// Reader goroutine
	go func() {
		for i := 0; i < 100; i++ {
			provider.Get(ctx, "concurrent-key")
		}
		done <- true
	}()

	// Deleter goroutine
	go func() {
		for i := 0; i < 100; i++ {
			provider.Delete(ctx, "concurrent-key")
		}
		done <- true
	}()

	// Wait for all goroutines
	<-done
	<-done
	<-done
}

func TestModuleNew_DefaultOptions(t *testing.T) {
	mod := New()

	if mod.defaultTTL != 5*time.Minute {
		t.Errorf("default TTL should be 5m, got %v", mod.defaultTTL)
	}
}

func TestModuleNew_WithDefaultTTL(t *testing.T) {
	mod := New(WithDefaultTTL(10 * time.Minute))

	if mod.defaultTTL != 10*time.Minute {
		t.Errorf("TTL should be 10m, got %v", mod.defaultTTL)
	}
}

func TestModuleNew_WithProvider(t *testing.T) {
	customProvider := NewMemoryProvider()
	mod := New(WithProvider(customProvider))

	if mod.provider != customProvider {
		t.Error("custom provider should be set")
	}
}

func TestModule_Name(t *testing.T) {
	mod := New()
	if mod.Name() != "cache" {
		t.Errorf("Name() should return 'cache', got %q", mod.Name())
	}
}
