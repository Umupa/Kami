package design

import (
	"bufio"
	"bytes"
	"embed"
	"path"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
)

// The themes are Omarchy's (github.com/omacom/omarchy, themes/*/colors.toml,
// MIT; see themes/LICENSE): each names its colors by role, which Palette
// maps to the panel's.
//
//go:embed themes/*.toml
var themeFiles embed.FS

// Themes are the palettes by name, and ThemeNames their names in order.
var (
	Themes     = map[string]*Palette{}
	ThemeNames []string
)

func init() {
	entries, _ := themeFiles.ReadDir("themes")
	for _, e := range entries {
		data, err := themeFiles.ReadFile("themes/" + e.Name())
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), path.Ext(e.Name()))
		Themes[name] = paletteOf(parseColors(data))
		ThemeNames = append(ThemeNames, name)
	}
	slices.Sort(ThemeNames)
}

// The themes "auto" picks by the system's appearance.
const (
	AutoDark  = "tokyo-night"
	AutoLight = "catppuccin-latte"
)

// For returns the palette of a theme name; "auto" or an unknown name follows
// the appearance.
func For(theme string, dark bool) *Palette {
	if p, ok := Themes[theme]; ok {
		return p
	}
	if dark {
		return Themes[AutoDark]
	}
	return Themes[AutoLight]
}

// parseColors reads the key = "value" lines of a colors.toml.
func parseColors(data []byte) map[string]string {
	colors := map[string]string{}
	sc := bufio.NewScanner(bytes.NewReader(data))
	for sc.Scan() {
		key, value, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		colors[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"`)
	}
	return colors
}

// paletteOf maps a theme's colors to the panel's roles.
func paletteOf(c map[string]string) *Palette {
	col := func(keys ...string) ui.Color {
		for _, k := range keys {
			if v, ok := c[k]; ok && strings.HasPrefix(v, "#") {
				return ui.Hex(v)
			}
		}
		return ui.Hex("#808080")
	}
	bg, fg := col("background"), col("foreground")
	p := &Palette{
		Dark:       c["mode"] != "light",
		Background: bg,
		Surface:    col("dark_background", "background"),
		Raised:     col("selection", "lighter_background"),
		Border:     bg.Mix(fg, 0.13),
		Text:       fg,
		Bright:     col("bright_foreground", "foreground"),
		Muted:      fg.Mix(bg, 0.32),
		Faint:      fg.Mix(bg, 0.55),
		Accent:     col("accent", "blue"),
		AccentText: bg,
		Link:       col("cyan", "blue"),
		Code:       col("green"),
		Quote:      col("magenta"),
		Operator:   col("bright_cyan", "cyan"),
		Sparkle:    col("yellow"),
		Danger:     col("red"),
		Keyword:    col("magenta"),
		String:     col("green"),
		Number:     col("orange", "bright_yellow", "yellow"),
		Comment:    col("dark_foreground"),
		Function:   col("blue"),
		Type:       col("cyan"),
	}
	p.Edge = p.Accent.Alpha(0.35)
	p.OperatorFill = p.Operator.Alpha(0.12)
	return p
}
