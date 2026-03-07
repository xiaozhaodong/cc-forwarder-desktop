package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveStartupConfigPath_UsesExplicitPathWhenProvided(t *testing.T) {
	tempDir := t.TempDir()
	explicitPath := filepath.Join(tempDir, "custom.yaml")
	resolvedPath, created, err := resolveStartupConfigPath(explicitPath, true, filepath.Join(tempDir, "config"), []byte("embedded"))
	if err != nil {
		t.Fatalf("resolveStartupConfigPath failed: %v", err)
	}
	if created {
		t.Fatal("expected explicit config path to skip default file creation")
	}
	if resolvedPath != explicitPath {
		t.Fatalf("expected explicit path %q, got %q", explicitPath, resolvedPath)
	}
	if _, err := os.Stat(filepath.Join(tempDir, "config", "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("expected default desktop config to remain absent, stat err=%v", err)
	}
}

func TestResolveStartupConfigPath_CreatesPersistentConfigWhenMissing(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	resolvedPath, created, err := resolveStartupConfigPath("", false, configDir, []byte("hello: world\n"))
	if err != nil {
		t.Fatalf("resolveStartupConfigPath failed: %v", err)
	}
	if !created {
		t.Fatal("expected config file to be created on first run")
	}
	wantPath := filepath.Join(configDir, "config.yaml")
	if resolvedPath != wantPath {
		t.Fatalf("expected resolved path %q, got %q", wantPath, resolvedPath)
	}
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		t.Fatalf("read created config failed: %v", err)
	}
	if string(content) != "hello: world\n" {
		t.Fatalf("unexpected created config content: %q", string(content))
	}
}

func TestResolveStartupConfigPath_PreservesExistingPersistentConfig(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, "config")
	configPath := filepath.Join(configDir, "config.yaml")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir failed: %v", err)
	}
	if err := os.WriteFile(configPath, []byte("existing: true\n"), 0o644); err != nil {
		t.Fatalf("seed existing config failed: %v", err)
	}
	resolvedPath, created, err := resolveStartupConfigPath("", false, configDir, []byte("embedded: true\n"))
	if err != nil {
		t.Fatalf("resolveStartupConfigPath failed: %v", err)
	}
	if created {
		t.Fatal("expected existing config file to be preserved")
	}
	if resolvedPath != configPath {
		t.Fatalf("expected resolved path %q, got %q", configPath, resolvedPath)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read existing config failed: %v", err)
	}
	if string(content) != "existing: true\n" {
		t.Fatalf("expected existing content to stay unchanged, got %q", string(content))
	}
}
