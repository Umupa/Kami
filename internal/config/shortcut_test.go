package config

import "testing"

func TestParseShortcutAccepts(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"none", "none"},
		{"double-option", "double-option"},
		{"double-command", "double-command"},
		{"control+option+a", "control+option+a"},
		// The canonical order is control, option, shift, command, so this
		// accepted spelling is rewritten.
		{"command+shift+5", "shift+command+5"},
		{"option+space", "option+space"},
		{"control+f12", "control+f12"},
		{"control+f20", "control+f20"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			sc, err := ParseShortcut(tt.input)
			if err != nil {
				t.Fatalf("ParseShortcut(%q): %v", tt.input, err)
			}
			if got := sc.String(); got != tt.want {
				t.Errorf("ParseShortcut(%q).String() = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestShortcutStringCanonicalOrder(t *testing.T) {
	sc, err := ParseShortcut("command+control+a")
	if err != nil {
		t.Fatalf("ParseShortcut: %v", err)
	}
	if sc.Mods != ModControl|ModCommand {
		t.Errorf("Mods = %b, want control and command", sc.Mods)
	}
	if got := sc.String(); got != "control+command+a" {
		t.Errorf("String() = %q, want control+command+a", got)
	}
}

func TestParseShortcutRejects(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"key only", "a"},
		{"shift only", "shift+a"},
		{"unknown double tap", "double-a"},
		{"duplicate modifier", "option+option+a"},
		{"abbreviated modifier", "opt+a"},
		{"uppercase key", "control+A"},
		{"function key zero", "control+f0"},
		{"function key too high", "control+f21"},
		{"function key leading zero", "control+f01"},
		{"unsupported key", "control+enter"},
		{"missing key", "control+"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc, err := ParseShortcut(tt.input)
			if err == nil {
				t.Fatalf("ParseShortcut(%q) = %+v, want an error", tt.input, sc)
			}
			if sc != (Shortcut{}) {
				t.Errorf("ParseShortcut(%q) = %+v, want the zero Shortcut", tt.input, sc)
			}
		})
	}
}
