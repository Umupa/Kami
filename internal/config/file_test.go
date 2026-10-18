package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareEmptyDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "userdata")
	if err := Prepare(dir); err != nil {
		t.Fatalf("Prepare: %v", err)
	}

	configData, err := os.ReadFile(filepath.Join(dir, FileName))
	if err != nil {
		t.Fatalf("read %s: %v", FileName, err)
	}
	cfg, err := Decode(configData)
	if err != nil {
		t.Fatalf("Decode prepared config: %v", err)
	}
	if cfg.Schema != "./"+SchemaFileName {
		t.Errorf("Schema = %q, want %q", cfg.Schema, "./"+SchemaFileName)
	}
	if cfg.Version != Version {
		t.Errorf("Version = %d, want %d", cfg.Version, Version)
	}

	schemaData, err := os.ReadFile(filepath.Join(dir, SchemaFileName))
	if err != nil {
		t.Fatalf("read %s: %v", SchemaFileName, err)
	}
	if !bytes.Equal(schemaData, Schema()) {
		t.Error("prepared schema differs from Schema()")
	}
}

func TestPrepareKeepsExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	existing := []byte(`{"version":1,"myLanguage":"请勿覆盖"}` + "\n")
	if err := os.WriteFile(path, existing, 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}

	if err := Prepare(dir); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !bytes.Equal(got, existing) {
		t.Errorf("Prepare rewrote config.json: got %q, want %q", got, existing)
	}
}

func TestWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, FileName)
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write old file: %v", err)
	}

	data := []byte("{\n  \"version\": 1\n}\n")
	if err := WriteFile(path, data); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != FileName {
		t.Errorf("dir entries = %v, want only %s", entries, FileName)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode = %o, want 600", got)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Errorf("content = %q, want %q", got, data)
	}
}
