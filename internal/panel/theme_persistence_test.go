package panel

import (
	"os"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/mode"
)

func TestThemePersistsWithModelError(t *testing.T) {
	sh := newSetupHarness(t, script{}, `{"version":1,"defaultModel":"old","models":[]}`)
	store := sh.p.deps.Store
	store.Subscribe(func(s config.Snapshot) {
		sh.p.post(func() { sh.p.applyConfig(s.Config) })
	})
	sh.p.deps.Operator.Apply = func(ops []config.Op) ([]config.Change, error) {
		data, err := os.ReadFile(store.Path())
		if err != nil {
			return nil, err
		}
		out, changes, err := config.ApplyOps(data, ops, mode.Check)
		if err != nil {
			return nil, err
		}
		if err := config.WriteFile(store.Path(), out); err != nil {
			return nil, err
		}
		store.Reload()
		return changes, nil
	}
	sh.show()
	for _, theme := range []string{"catppuccin-latte", "gruvbox"} {
		sh.tt.Click("原文")
		sh.tt.Type("/theme " + theme)
		sh.tt.Frame()
		sh.tt.Key(0, ui.KeyEnter)
		sh.drain()
		if sh.p.cfg.Theme != theme || sh.p.notice != "" {
			t.Fatalf("theme = %q, notice = %q", sh.p.cfg.Theme, sh.p.notice)
		}
		if snap := store.Snapshot(); snap.Config.Theme != theme || snap.Err == nil {
			t.Fatal("theme reload failed or model error was hidden")
		}
		if snap := config.NewStore(store.Path(), mode.Check).Snapshot(); snap.Config.Theme != theme || snap.Err == nil {
			t.Fatal("theme did not survive restart")
		}
	}
	sh.snapshot("theme-with-model-error")
}
