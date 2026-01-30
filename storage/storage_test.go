package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalProvider_PutAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &LocalProvider{basePath: tmpDir}
	ctx := context.Background()

	key := "test/file.txt"
	data := []byte("hello world")

	err := provider.Put(ctx, key, data)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := provider.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(got) != string(data) {
		t.Errorf("Get returned %q, want %q", got, data)
	}
}

func TestLocalProvider_GetNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &LocalProvider{basePath: tmpDir}
	ctx := context.Background()

	_, err := provider.Get(ctx, "nonexistent.txt")
	if !os.IsNotExist(err) {
		t.Errorf("expected os.ErrNotExist, got: %v", err)
	}
}

func TestLocalProvider_Delete(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &LocalProvider{basePath: tmpDir}
	ctx := context.Background()

	key := "to-delete.txt"
	data := []byte("delete me")

	provider.Put(ctx, key, data)

	err := provider.Delete(ctx, key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = provider.Get(ctx, key)
	if !os.IsNotExist(err) {
		t.Error("file should not exist after delete")
	}
}

func TestLocalProvider_DeleteNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &LocalProvider{basePath: tmpDir}
	ctx := context.Background()

	// Deleting nonexistent file should not error
	err := provider.Delete(ctx, "nonexistent.txt")
	if err != nil {
		t.Errorf("Delete of nonexistent file should not error, got: %v", err)
	}
}

func TestLocalProvider_List(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &LocalProvider{basePath: tmpDir}
	ctx := context.Background()

	// Create some files
	files := []string{
		"docs/readme.txt",
		"docs/guide.txt",
		"images/logo.png",
		"root.txt",
	}

	for _, f := range files {
		provider.Put(ctx, f, []byte("content"))
	}

	// List with prefix
	keys, err := provider.List(ctx, "docs/")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(keys) != 2 {
		t.Errorf("expected 2 keys with prefix 'docs/', got %d: %v", len(keys), keys)
	}
}

func TestLocalProvider_ListEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &LocalProvider{basePath: tmpDir}
	ctx := context.Background()

	keys, err := provider.List(ctx, "nonexistent/")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(keys) != 0 {
		t.Errorf("expected 0 keys, got %d", len(keys))
	}
}

func TestLocalProvider_PutCreatesDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &LocalProvider{basePath: tmpDir}
	ctx := context.Background()

	// Deep nested path
	key := "a/b/c/d/deep-file.txt"
	data := []byte("deep content")

	err := provider.Put(ctx, key, data)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify file exists at expected path
	fullPath := filepath.Join(tmpDir, key)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Error("file was not created at expected path")
	}
}

func TestLocalProvider_PutOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &LocalProvider{basePath: tmpDir}
	ctx := context.Background()

	key := "overwrite.txt"

	provider.Put(ctx, key, []byte("first"))
	provider.Put(ctx, key, []byte("second"))

	got, _ := provider.Get(ctx, key)
	if string(got) != "second" {
		t.Errorf("expected 'second', got %q", got)
	}
}

func TestModule_Name(t *testing.T) {
	mod := New()
	if mod.Name() != "storage" {
		t.Errorf("Name() should return 'storage', got %q", mod.Name())
	}
}

func TestModuleNew_DefaultOptions(t *testing.T) {
	mod := New()
	if mod.basePath != "./data/storage" {
		t.Errorf("default basePath should be './data/storage', got %q", mod.basePath)
	}
}

func TestModuleNew_WithBasePath(t *testing.T) {
	mod := New(WithBasePath("/custom/path"))
	if mod.basePath != "/custom/path" {
		t.Errorf("basePath should be '/custom/path', got %q", mod.basePath)
	}
	if !mod.basePathFromOpt {
		t.Error("basePathFromOpt should be true")
	}
}

func TestModuleNew_WithProvider(t *testing.T) {
	customProvider := &LocalProvider{basePath: "/custom"}
	mod := New(WithProvider(customProvider))

	if mod.provider != customProvider {
		t.Error("custom provider should be set")
	}
}

// Integration test using Module methods
func TestModule_Integration(t *testing.T) {
	tmpDir := t.TempDir()
	provider := &LocalProvider{basePath: tmpDir}
	mod := New(WithProvider(provider))
	ctx := context.Background()

	// Put
	err := mod.Put(ctx, "test.txt", []byte("hello"))
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Get
	data, err := mod.Get(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if string(data) != "hello" {
		t.Errorf("expected 'hello', got %q", data)
	}

	// List
	keys, err := mod.List(ctx, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}

	// Delete
	err = mod.Delete(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify deleted
	_, err = mod.Get(ctx, "test.txt")
	if !os.IsNotExist(err) {
		t.Error("file should not exist after delete")
	}
}
