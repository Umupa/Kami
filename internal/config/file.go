package config

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// File names inside the app's user data directory.
const (
	FileName       = "config.json"
	SchemaFileName = "config.schema.json"
)

//go:embed schema.json
var schema []byte

// Schema returns the JSON Schema of config.json.
func Schema() []byte { return schema }

// starter is what a missing config.json starts as: valid, and pointing editors
// and agents at the schema.
var starter = Config{Schema: "./" + SchemaFileName, Version: Version}

// Prepare makes dir ready for an agent: it writes the schema of this build,
// and a starter config.json when there is none. It never overwrites a config.
func Prepare(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := writeIfChanged(filepath.Join(dir, SchemaFileName), schema); err != nil {
		return err
	}
	path := filepath.Join(dir, FileName)
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	data, err := Encode(&starter)
	if err != nil {
		return err
	}
	return WriteFile(path, data)
}

// Encode formats c the way the app writes config.json.
func Encode(c *Config) ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// WriteFile replaces path atomically, so that the watcher and other readers
// never see half a file.
func WriteFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replace %s: %w", filepath.Base(path), err)
	}
	return nil
}

func writeIfChanged(path string, data []byte) error {
	if old, err := os.ReadFile(path); err == nil && string(old) == string(data) {
		return nil
	}
	return WriteFile(path, data)
}

// Edit changes config.json with fn, as the settings window does. It reads the
// file as written, without defaults, so that only what fn changes is written
// back; the result must be valid, or nothing is written.
func Edit(path string, fn func(*Config)) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return &Error{Problems: []Problem{{Message: syntaxMessage(data, err)}}}
	}
	fn(&c)
	out, err := Encode(&c)
	if err != nil {
		return err
	}
	if _, err := Decode(out); err != nil {
		return err
	}
	return WriteFile(path, out)
}
