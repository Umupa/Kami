package config

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestNewStoreValidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	writeConfigFile(t, path, `{"version":1,"myLanguage":"日本語"}`)

	store := NewStore(path, nil)
	snap := store.Snapshot()
	if snap.Err != nil {
		t.Fatalf("Snapshot().Err = %v, want nil", snap.Err)
	}
	if snap.Config == nil {
		t.Fatal("Snapshot().Config = nil, want a config")
	}
	if snap.Config.MyLanguage != "日本語" {
		t.Errorf("MyLanguage = %q, want 日本語", snap.Config.MyLanguage)
	}
	if store.Path() != path {
		t.Errorf("Path() = %q, want %q", store.Path(), path)
	}
}

func TestNewStoreMissingFile(t *testing.T) {
	store := NewStore(filepath.Join(t.TempDir(), FileName), nil)
	snap := store.Snapshot()
	if snap.Err == nil {
		t.Fatal("Snapshot().Err = nil, want an error for a missing file")
	}
	if snap.Config == nil {
		t.Fatal("Snapshot().Config = nil, want the starter configuration")
	}
	if snap.Config.Version != Version {
		t.Errorf("Version = %d, want %d", snap.Config.Version, Version)
	}
	if snap.Config.Schema != "./"+SchemaFileName {
		t.Errorf("Schema = %q, want %q", snap.Config.Schema, "./"+SchemaFileName)
	}
	if snap.Config.Shortcuts.Panel != DefaultPanelShortcut {
		t.Errorf("Panel = %q, want %q", snap.Config.Shortcuts.Panel, DefaultPanelShortcut)
	}
}

func TestStoreReloadKeepsLastValidConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	writeConfigFile(t, path, `{"version":1,"myLanguage":"first"}`)

	store := NewStore(path, nil)
	first := store.Snapshot().Config
	if first == nil {
		t.Fatal("initial Config = nil")
	}

	writeConfigFile(t, path, `{"version":2}`)
	snap := store.Reload()
	if snap.Err == nil {
		t.Fatal("Reload() after invalid file: Err = nil, want an error")
	}
	if snap.Config != first {
		t.Error("Reload() replaced the config after an invalid file")
	}

	writeConfigFile(t, path, `{"version":1,"myLanguage":"second"}`)
	snap = store.Reload()
	if snap.Err != nil {
		t.Fatalf("Reload() after valid file: Err = %v, want nil", snap.Err)
	}
	if snap.Config == first {
		t.Error("Reload() kept the old config after a valid file")
	}
	if snap.Config.MyLanguage != "second" {
		t.Errorf("MyLanguage = %q, want second", snap.Config.MyLanguage)
	}
}

func TestStoreSubscribers(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	writeConfigFile(t, path, `{"version":1,"myLanguage":"A"}`)

	store := NewStore(path, nil)
	calls := 0
	var last Snapshot
	cancel := store.Subscribe(func(snap Snapshot) {
		calls++
		last = snap
	})

	writeConfigFile(t, path, `{"version":1,"myLanguage":"B"}`)
	store.Reload()
	if calls != 1 {
		t.Fatalf("subscriber calls = %d after one change, want 1", calls)
	}
	if last.Err != nil || last.Config.MyLanguage != "B" {
		t.Errorf("subscriber got %+v", last)
	}

	// Same bytes, so nothing changed even though the file was rewritten.
	writeConfigFile(t, path, `{"version":1,"myLanguage":"B"}`)
	store.Reload()
	if calls != 1 {
		t.Errorf("subscriber calls = %d after identical bytes, want 1", calls)
	}

	writeConfigFile(t, path, `{"version":1,"myLanguage":"C"}`)
	store.Reload()
	if calls != 2 {
		t.Errorf("subscriber calls = %d after a second change, want 2", calls)
	}

	cancel()
	writeConfigFile(t, path, `{"version":1,"myLanguage":"D"}`)
	store.Reload()
	if calls != 2 {
		t.Errorf("subscriber calls = %d after cancel, want 2", calls)
	}
	if got := store.Snapshot().Config.MyLanguage; got != "D" {
		t.Errorf("MyLanguage after cancel = %q, want D", got)
	}
}

func TestStoreCheckHook(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	writeConfigFile(t, path, `{"version":1,"myLanguage":"first"}`)

	var checkErr error
	store := NewStore(path, func(*Config) error { return checkErr })
	first := store.Snapshot().Config
	if first == nil {
		t.Fatal("initial Config = nil")
	}

	sentinel := errors.New("mode check failed")
	checkErr = sentinel
	writeConfigFile(t, path, `{"version":1,"myLanguage":"rejected"}`)
	snap := store.Reload()
	if snap.Config != first {
		t.Error("check failure replaced the config")
	}
	if !errors.Is(snap.Err, sentinel) {
		t.Errorf("Err = %v, want %v", snap.Err, sentinel)
	}

	checkErr = nil
	writeConfigFile(t, path, `{"version":1,"myLanguage":"accepted"}`)
	snap = store.Reload()
	if snap.Err != nil {
		t.Fatalf("Err after passing check = %v, want nil", snap.Err)
	}
	if snap.Config.MyLanguage != "accepted" {
		t.Errorf("MyLanguage = %q, want accepted", snap.Config.MyLanguage)
	}
}

func TestStoreConcurrentReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	writeConfigFile(t, path, `{"version":1,"myLanguage":"A"}`)
	store := NewStore(path, nil)

	contents := []string{
		`{"version":1,"myLanguage":"A"}`,
		`{"version":1,"myLanguage":"B"}`,
		`{"version":2}`,
	}
	const readers = 8
	const iterations = 50

	done := make(chan struct{})
	var writer sync.WaitGroup
	writer.Add(1)
	var writeErr error
	go func() {
		defer writer.Done()
		for i := 0; ; i++ {
			select {
			case <-done:
				return
			default:
			}
			if err := os.WriteFile(path, []byte(contents[i%len(contents)]), 0o600); err != nil {
				writeErr = err
				return
			}
		}
	}()

	var group sync.WaitGroup
	for i := 0; i < readers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			for j := 0; j < iterations; j++ {
				store.Reload()
			}
		}()
	}
	group.Wait()
	close(done)
	writer.Wait()

	if writeErr != nil {
		t.Fatalf("rewriting the file: %v", writeErr)
	}
	if snap := store.Snapshot(); snap.Config == nil {
		t.Fatal("Snapshot().Config = nil after concurrent reloads")
	}
}

func writeConfigFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
