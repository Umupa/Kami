package app

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/egoist/mygo"

	"github.com/Umupa/Kami/internal/config"
)

func TestDockMenuPersistsWithoutModel(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(path, []byte(`{"version":1,"model":false}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var policies []mygo.ActivationPolicy
	a := &app{
		store: config.NewStore(path, nil), log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		setActivationPolicy: func(policy mygo.ActivationPolicy) { policies = append(policies, policy) },
	}
	a.menu = mygo.NewMenu([]*mygo.MenuItem{a.dockMenuItem()})
	item := a.menu.ItemByID("show-in-dock")
	a.applyDock(a.store.Snapshot().Config)
	if item.Type != mygo.MenuItemNormal || item.IsChecked() || item.Label != "显示 Dock 图标" || len(policies) != 0 {
		t.Fatal("Dock should start hidden without changing the startup policy")
	}
	for _, visible := range []bool{true, false} {
		item.Click(item, nil)
		wantLabel := "显示 Dock 图标"
		if visible {
			wantLabel = "隐藏 Dock 图标"
		}
		if item.IsChecked() || item.Label != wantLabel || a.dockVisible != visible {
			t.Fatal("menu action and Dock visibility disagree")
		}
		if snap := config.NewStore(path, nil).Snapshot(); snap.Config.ShowInDock != visible || snap.Err == nil {
			t.Fatal("Dock setting did not persist, or model error was lost")
		}
		// Reloading unrelated settings must not toggle the native policy again.
		a.applyDock(a.store.Snapshot().Config)
	}
	if !slices.Equal(policies, []mygo.ActivationPolicy{mygo.ActivationPolicyRegular, mygo.ActivationPolicyAccessory}) {
		t.Fatalf("policies = %v", policies)
	}
	// Undo follows the same hot-load path as a manual edit or Operator change.
	if !a.undoOps() {
		t.Fatal("undo failed")
	}
	a.applyDock(a.store.Snapshot().Config)
	if !a.dockVisible || item.IsChecked() || item.Label != "隐藏 Dock 图标" {
		t.Fatal("undo did not restore Dock visibility")
	}
	// A failed save must keep the action label and Dock visibility unchanged.
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	count := len(policies)
	item.Click(item, nil)
	if item.IsChecked() || item.Label != "隐藏 Dock 图标" || !a.dockVisible || len(policies) != count {
		t.Fatal("failed save changed the Dock or left a misleading action")
	}
}

func TestDockRestoresSavedVisibility(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(path, []byte(`{"version":1,"showInDock":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var got mygo.ActivationPolicy
	a := &app{store: config.NewStore(path, nil), setActivationPolicy: func(policy mygo.ActivationPolicy) { got = policy }}
	a.menu = mygo.NewMenu([]*mygo.MenuItem{a.dockMenuItem()})
	a.applyDock(a.store.Snapshot().Config)
	if got != mygo.ActivationPolicyRegular || a.menu.ItemByID("show-in-dock").Label != "隐藏 Dock 图标" {
		t.Fatal("saved Dock visibility was not restored")
	}
}
