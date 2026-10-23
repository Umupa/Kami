package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSettingsEditsPreserveInvalidModels(t *testing.T) {
	for _, fields := range []string{
		`"model":{"endpoint":"broken","format":"chat","model":"","apiKey":"test-secret","body":{"seed":9007199254740993}}`,
		`"model":"wrong type"`,
		`"model":{"unknown":"field"}`,
		`"defaultModel":"old","models":[{"id":"old","apiKey":"test-secret","body":{"seed":9007199254740993}}]`,
	} {
		input := []byte(`{"version":1,` + fields + `}`)
		out, changes, err := ApplyOps(input, []Op{
			{Path: "theme", Value: json.RawMessage(`"gruvbox"`)},
			{Path: "fontSize", Value: json.RawMessage(`16`)},
			{Path: "background", Value: json.RawMessage(`"solid"`)},
			{Path: "shortcuts.panel", Value: json.RawMessage(`"control+option+p"`)},
		}, nil)
		if err != nil {
			t.Fatalf("model error blocked settings: %v", err)
		}
		if len(changes) != 4 {
			t.Fatalf("got %d changes, want 4", len(changes))
		}
		before, _ := decodeJSON(input)
		after, _ := decodeJSON(out)
		for _, key := range []string{"model", "models", "defaultModel"} {
			b, _ := json.Marshal(before.(map[string]any)[key])
			a, _ := json.Marshal(after.(map[string]any)[key])
			if !bytes.Equal(a, b) {
				t.Errorf("settings edit changed %s", key)
			}
		}
		if _, err := Decode(out); err == nil {
			t.Error("strict validation stopped reporting the model error")
		}
		for _, change := range changes {
			if strings.Contains(change.Before+change.After, "test-secret") {
				t.Error("settings change report exposed the model key")
			}
		}
	}
}

func TestInvalidModelDoesNotBypassSettingsValidation(t *testing.T) {
	for _, op := range []Op{
		{Path: "theme", Value: json.RawMessage(`"not-a-theme"`)},
		{Path: "fontSize", Value: json.RawMessage(`100`)},
		{Path: "shortcuts.panel", Value: json.RawMessage(`"option+s"`)},
		{Path: "unknown", Value: json.RawMessage(`true`)},
		{Path: "models[0].apiKey", Value: json.RawMessage(`"replacement"`)},
		{Path: "defaultModel", Value: json.RawMessage(`null`)},
	} {
		out, changes, err := ApplyOps([]byte(`{"version":1,"model":false,"models":[{"apiKey":"test-secret"}],"defaultModel":"old"}`), []Op{op}, nil)
		if err == nil || out != nil || changes != nil {
			t.Errorf("invalid edit %s was accepted", op.Path)
		}
	}
}

func TestStoreLoadsSettingsWithInvalidModel(t *testing.T) {
	for _, initial := range []string{
		`{"version":1}`,
		`{"version":1,"model":{"endpoint":"https://example.com","format":"chat-completions","model":"working"}}`,
	} {
		path := filepath.Join(t.TempDir(), FileName)
		writeConfigFile(t, path, initial)
		store := NewStore(path, nil)
		previous := store.Snapshot().Config.Model
		calls := 0
		store.Subscribe(func(Snapshot) { calls++ })
		for _, fields := range []string{`"model":false`, `"models":[],"defaultModel":"old"`} {
			writeConfigFile(t, path, `{"version":1,"theme":"gruvbox","fontSize":16,`+fields+`}`)
			snap := store.Reload()
			if snap.Err == nil || snap.Config.Theme != "gruvbox" || snap.Config.FontSize != 16 || snap.Config.Model != previous {
				t.Fatal("model error did not preserve the last model and load valid settings")
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			out, _, err := ApplyOps(data, []Op{{Path: "theme", Value: json.RawMessage(`"kanagawa"`)}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := WriteFile(path, out); err != nil {
				t.Fatal(err)
			}
			if got := store.Reload(); got.Err == nil || got.Config.Theme != "kanagawa" || got.Config.Model != previous {
				t.Fatal("saved theme did not survive reload")
			}
			if got := NewStore(path, nil).Snapshot(); got.Err == nil || got.Config.Theme != "kanagawa" || got.Config.Model != nil {
				t.Fatal("saved theme did not survive restart, or invalid model was loaded")
			}
		}
		if calls != 4 {
			t.Errorf("got %d notifications, want 4", calls)
		}
		writeConfigFile(t, path, initial)
		if got := store.Reload(); got.Err != nil || got.Config.Theme != DefaultTheme {
			t.Fatal("model repair did not clear the error")
		}
	}
}

func TestStoreModelFallbackStillValidatesSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), FileName)
	writeConfigFile(t, path, `{"version":1,"theme":"gruvbox"}`)
	var checkErr error
	store := NewStore(path, func(*Config) error { return checkErr })
	previous := store.Snapshot().Config
	writeConfigFile(t, path, `{"version":1,"model":false,"theme":"not-a-theme"}`)
	if got := store.Reload(); got.Err == nil || got.Config != previous {
		t.Fatal("model fallback bypassed settings validation")
	}
	checkErr = errors.New("custom mode is incomplete")
	writeConfigFile(t, path, `{"version":1,"model":false,"theme":"kanagawa"}`)
	if got := store.Reload(); !errors.Is(got.Err, checkErr) || got.Config != previous {
		t.Fatal("model fallback bypassed the settings check hook")
	}
}
