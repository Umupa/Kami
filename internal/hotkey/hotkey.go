// Package hotkey registers the app's global shortcuts from the configuration,
// and registers them again when it changes.
package hotkey

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/egoist/mygo"

	"github.com/Umupa/cida-next/internal/config"
)

// Binding is a shortcut and what it does.
type Binding struct {
	Name     string // the config field, for logs
	Shortcut string
	Action   func()
}

// Manager owns the registered shortcuts. Apply runs on the main thread.
type Manager struct {
	log        *slog.Logger
	registered []string
}

// NewManager returns a Manager that registers nothing yet.
func NewManager(log *slog.Logger) *Manager { return &Manager{log: log} }

// Apply replaces the registered shortcuts with bindings. A shortcut that
// cannot be registered, as one another app holds, is logged and skipped.
func (m *Manager) Apply(bindings []Binding) {
	for _, acc := range m.registered {
		mygo.GlobalShortcut.Unregister(acc)
	}
	m.registered = m.registered[:0]
	for _, b := range bindings {
		sc, err := config.ParseShortcut(b.Shortcut)
		if err != nil || sc.IsZero() {
			continue
		}
		if sc.Double != 0 {
			// Double taps are not hotkeys; the event tap watches for them.
			continue
		}
		acc := Accelerator(sc)
		if err := mygo.GlobalShortcut.Register(acc, b.Action); err != nil {
			m.log.Warn("register shortcut", "name", b.Name, "shortcut", b.Shortcut, "err", err)
			continue
		}
		m.log.Info("registered shortcut", "name", b.Name, "accelerator", acc)
		m.registered = append(m.registered, acc)
	}
}

// Accelerator spells a key combination as MyGo's global shortcuts take it.
func Accelerator(sc config.Shortcut) string {
	var parts []string
	for _, m := range []struct {
		mod  config.Modifiers
		name string
	}{
		{config.ModControl, "Ctrl"},
		{config.ModOption, "Alt"},
		{config.ModShift, "Shift"},
		{config.ModCommand, "Cmd"},
	} {
		if sc.Mods&m.mod != 0 {
			parts = append(parts, m.name)
		}
	}
	key := sc.Key
	if len(key) == 1 {
		key = strings.ToUpper(key)
	} else {
		key = fmt.Sprintf("%s%s", strings.ToUpper(key[:1]), key[1:])
	}
	return strings.Join(append(parts, key), "+")
}
