package settings

import (
	"encoding/json"
	"flag"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/keyring"
	"github.com/Umupa/cida-next/internal/mode"
)

var update = flag.Bool("update", false, "rewrite the snapshots in testdata")

const testConfig = `{
  "version": 1,
  "models": [
    {"name": "fast", "endpoint": "https://api.example.com/v1/chat/completions", "format": "chat-completions", "model": "small-1"},
    {"name": "strong", "endpoint": "https://api.example.com/v1/messages", "format": "anthropic-messages", "model": "large-2"}
  ]
}`

func newTestSettings(t *testing.T) (*Settings, string, *bool) {
	t.Helper()
	path := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(path, []byte(testConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	store := config.NewStore(path, mode.Check)
	paused := false
	s := &Settings{
		deps: Deps{
			Store:        store,
			Keyring:      keyring.NewMemory(),
			Log:          slog.New(slog.NewTextHandler(io.Discard, nil)),
			CopyPrompt:   func() {},
			RevealConfig: func() {},
			PauseHotkeys: func(p bool) { paused = p },
			Permissions: []Permission{
				{Name: "辅助功能", Purpose: "带入选中的文字", Granted: func() bool { return true }, Request: func() {}},
				{Name: "屏幕录制", Purpose: "截图识别", Granted: func() bool { return false }, Request: func() {}},
			},
		},
		keys:    map[string]bool{"fast": true},
		checks:  map[string]check{},
		snap:    store.Snapshot(),
		version: "0.1.0",
	}
	return s, path, &paused
}

func snapshot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	if !*update {
		return
	}
	os.MkdirAll("testdata", 0o755)
	f, err := os.Create(filepath.Join("testdata", name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	png.Encode(f, tt.Image())
}

func TestTabs(t *testing.T) {
	s, _, _ := newTestSettings(t)
	tt := ui.NewTester(s.view, 680, 520)
	for i, name := range []string{"models", "modes", "shortcuts", "general"} {
		s.tab = i
		tt.Frame()
		snapshot(t, tt, name)
	}
	s.tab = 0
	tt.Frame()
	if !tt.HasText("API Key 已保存在钥匙串") || !tt.HasText("small-1 · chat-completions · api.example.com") {
		t.Errorf("models tab shows %q", tt.Texts())
	}
}

func TestRecordShortcut(t *testing.T) {
	s, path, paused := newTestSettings(t)
	tt := ui.NewTester(s.view, 680, 520)
	s.tab = 2
	tt.Frame()
	s.startRecording("selection")
	tt.Frame()
	if !*paused {
		t.Error("global shortcuts not paused while recording")
	}
	tt.Key(ui.Ctrl|ui.Alt, ui.KeyT)
	if *paused || s.recording != "" {
		t.Error("recording did not end")
	}
	data, _ := os.ReadFile(path)
	c, err := config.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if c.Shortcuts.Selection != "control+option+t" {
		t.Errorf("selection = %q", c.Shortcuts.Selection)
	}
	if c.Models[0].Auth != "bearer" {
		t.Fatal("decode should fill defaults")
	}
	// Only the changed field is written back, not the defaults.
	var raw config.Config
	if err := jsonUnmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw.Models[0].Auth != "" || raw.Shortcuts.Panel != "" {
		t.Errorf("defaults were written back: %s", data)
	}
}

func TestEscapeCancelsRecording(t *testing.T) {
	s, path, paused := newTestSettings(t)
	tt := ui.NewTester(s.view, 680, 520)
	s.tab = 2
	tt.Frame()
	s.startRecording("capture")
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if *paused || s.recording != "" {
		t.Error("Escape did not cancel")
	}
	data, _ := os.ReadFile(path)
	if string(data) != testConfig {
		t.Error("cancelling changed the file")
	}
}

func TestCombinationAndGlyphs(t *testing.T) {
	tests := []struct {
		key  ui.Key
		mods ui.Modifiers
		want string
		ok   bool
	}{
		{ui.KeyA, ui.Ctrl | ui.Alt, "control+option+a", true},
		{ui.Key5, ui.Super | ui.Shift, "shift+command+5", true},
		{ui.KeySpace, ui.Alt, "option+space", true},
		{ui.KeyF5, ui.Ctrl, "control+f5", true},
		{ui.KeyA, 0, "", false},
		{ui.KeyA, ui.Shift, "", false},
		{ui.KeyEnter, ui.Ctrl, "", false},
	}
	for _, tt := range tests {
		got, ok := combination(tt.key, tt.mods)
		if got != tt.want || ok != tt.ok {
			t.Errorf("combination(%v, %v) = %q, %v; want %q, %v", tt.key, tt.mods, got, ok, tt.want, tt.ok)
		}
	}
	for in, want := range map[string]string{
		"control+option+a": "⌃⌥A", "double-option": "双击 ⌥", "none": "未设置", "shift+command+space": "⇧⌘SPACE",
	} {
		if got := Glyphs(in); got != want {
			t.Errorf("Glyphs(%q) = %q, want %q", in, got, want)
		}
	}
}

func jsonUnmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
