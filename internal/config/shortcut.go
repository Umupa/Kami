package config

import (
	"errors"
	"fmt"
	"strings"
)

// Modifiers is a set of modifier keys.
type Modifiers uint8

const (
	ModControl Modifiers = 1 << iota
	ModOption
	ModShift
	ModCommand
)

// modifierNames is in the order macOS writes modifiers: ⌃⌥⇧⌘.
var modifierNames = []struct {
	mod  Modifiers
	name string
}{
	{ModControl, "control"},
	{ModOption, "option"},
	{ModShift, "shift"},
	{ModCommand, "command"},
}

// Shortcut is a global shortcut: a key with modifiers, a modifier tapped
// twice, or nothing. The zero value is no shortcut.
type Shortcut struct {
	// Mods and Key are a key combination; Key is "a"–"z", "0"–"9",
	// "space" or "f1"–"f20".
	Mods Modifiers
	Key  string
	// Double is the one modifier of a double tap.
	Double Modifiers
}

// IsZero reports whether s is no shortcut.
func (s Shortcut) IsZero() bool { return s == Shortcut{} }

// String returns the canonical spelling ParseShortcut reads back.
func (s Shortcut) String() string {
	switch {
	case s.IsZero():
		return "none"
	case s.Double != 0:
		return "double-" + modifierName(s.Double)
	}
	var parts []string
	for _, m := range modifierNames {
		if s.Mods&m.mod != 0 {
			parts = append(parts, m.name)
		}
	}
	return strings.Join(append(parts, s.Key), "+")
}

func modifierName(mod Modifiers) string {
	for _, m := range modifierNames {
		if m.mod == mod {
			return m.name
		}
	}
	return ""
}

func modifierByName(name string) Modifiers {
	for _, m := range modifierNames {
		if m.name == name {
			return m.mod
		}
	}
	return 0
}

// ParseShortcut reads "none", "double-option" (or another modifier), or a
// combination such as "control+option+a". A combination needs a modifier
// other than shift, so that it never steals ordinary typing.
func ParseShortcut(s string) (Shortcut, error) {
	if s == "none" {
		return Shortcut{}, nil
	}
	if name, ok := strings.CutPrefix(s, "double-"); ok {
		mod := modifierByName(name)
		if mod == 0 {
			return Shortcut{}, fmt.Errorf("「%s」不是修饰键，可用 control、option、shift、command", name)
		}
		return Shortcut{Double: mod}, nil
	}
	parts := strings.Split(s, "+")
	key := parts[len(parts)-1]
	if !validKey(key) {
		return Shortcut{}, fmt.Errorf("「%s」不是支持的按键，可用 a–z、0–9、space、f1–f20", key)
	}
	var sc Shortcut
	for _, name := range parts[:len(parts)-1] {
		mod := modifierByName(name)
		if mod == 0 {
			return Shortcut{}, fmt.Errorf("「%s」不是修饰键，可用 control、option、shift、command", name)
		}
		if sc.Mods&mod != 0 {
			return Shortcut{}, fmt.Errorf("修饰键「%s」重复", name)
		}
		sc.Mods |= mod
	}
	if sc.Mods&^ModShift == 0 {
		return Shortcut{}, errors.New("组合键至少要有 control、option 或 command 之一")
	}
	sc.Key = key
	return sc, nil
}

func validKey(k string) bool {
	if len(k) == 1 {
		return k[0] >= 'a' && k[0] <= 'z' || k[0] >= '0' && k[0] <= '9'
	}
	if k == "space" {
		return true
	}
	var n int
	if _, err := fmt.Sscanf(k, "f%d", &n); err == nil && fmt.Sprintf("f%d", n) == k {
		return n >= 1 && n <= 20
	}
	return false
}

// Glyphs shows a shortcut as macOS menus do: ⌃⌥A, ⌥Space, 双击 ⌥, 未设置.
func (s Shortcut) Glyphs() string {
	switch {
	case s.IsZero():
		return "未设置"
	case s.Double != 0:
		return "双击 " + modifierGlyph(s.Double)
	}
	return strings.Join(s.Keys(), "")
}

// Keys are the keys of a shortcut as their keycaps read, modifiers first:
// ⌥ and Space for option+space; the one modifier of a double tap.
func (s Shortcut) Keys() []string {
	if s.Double != 0 {
		return []string{modifierGlyph(s.Double)}
	}
	var keys []string
	for _, m := range modifierNames {
		if s.Mods&m.mod != 0 {
			keys = append(keys, modifierGlyph(m.mod))
		}
	}
	switch {
	case s.Key == "":
	case s.Key == "space":
		keys = append(keys, "Space")
	default:
		keys = append(keys, strings.ToUpper(s.Key))
	}
	return keys
}

func modifierGlyph(mod Modifiers) string {
	return map[Modifiers]string{ModControl: "⌃", ModOption: "⌥", ModShift: "⇧", ModCommand: "⌘"}[mod]
}
