package design

import (
	"bufio"
	"bytes"
	"embed"
	"fmt"
	"math"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/egoist/mygo/ui"
)

// The themes are Omarchy's (github.com/omacom/omarchy, themes/*/colors.toml,
// MIT; see themes/LICENSE), and flexoki-night, Flexoki's dark colors
// (github.com/kepano/flexoki, MIT; see themes/LICENSE-flexoki) in the same
// roles: each names its colors by role, which Palette maps to the panel's.
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
	AutoDark  = "flexoki-night"
	AutoLight = "flexoki-light"
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
		Text:       fg,
		Bright:     col("bright_foreground", "foreground"),
		Accent:     col("accent", "blue"),
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
	p.derive()
	return p
}

func (p *Palette) derive() {
	p.Border = p.Background.Mix(p.Text, 0.13)
	p.Muted = p.Text.Mix(p.Background, 0.32)
	p.Faint = p.Text.Mix(p.Background, 0.55)
	p.AccentText = p.Background
	p.Edge = p.Accent.Alpha(0.35)
	p.OperatorFill = p.Operator.Alpha(0.12)
}

var hexColor = regexp.MustCompile(`^#([0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)

// Derive returns base with valid named colors replaced. Derived colors follow
// their sources unless explicitly supplied. Unknown names and bad values are ignored.
func Derive(base *Palette, colors map[string]string) *Palette {
	p := *base
	apply := func() {
		for name, field := range p.colorFields() {
			if value := colors[name]; hexColor.MatchString(value) {
				*field = ui.Hex(value)
			}
		}
	}
	apply()
	p.derive()
	apply()
	return &p
}

func (p *Palette) colorFields() map[string]*ui.Color {
	return map[string]*ui.Color{
		"background":   &p.Background,
		"surface":      &p.Surface,
		"raised":       &p.Raised,
		"border":       &p.Border,
		"edge":         &p.Edge,
		"text":         &p.Text,
		"bright":       &p.Bright,
		"muted":        &p.Muted,
		"faint":        &p.Faint,
		"accent":       &p.Accent,
		"accentText":   &p.AccentText,
		"link":         &p.Link,
		"code":         &p.Code,
		"quote":        &p.Quote,
		"operator":     &p.Operator,
		"operatorFill": &p.OperatorFill,
		"sparkle":      &p.Sparkle,
		"danger":       &p.Danger,
		"keyword":      &p.Keyword,
		"string":       &p.String,
		"number":       &p.Number,
		"comment":      &p.Comment,
		"function":     &p.Function,
		"type":         &p.Type,
	}
}

// Colors returns the palette's named colors as #RRGGBB or #RRGGBBAA.
func (p *Palette) Colors() map[string]string {
	colors := make(map[string]string)
	for name, c := range p.colorFields() {
		value := fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
		if c.A != 255 {
			value += fmt.Sprintf("%02X", c.A)
		}
		colors[name] = value
	}
	return colors
}

// Contrast is the WCAG contrast ratio of two opaque colors, from 1 to 21.
func Contrast(a, b ui.Color) float64 {
	linear := func(v uint8) float64 {
		c := float64(v) / 255
		if c <= 0.04045 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	luminance := func(c ui.Color) float64 { return 0.2126*linear(c.R) + 0.7152*linear(c.G) + 0.0722*linear(c.B) }
	x, y := luminance(a), luminance(b)
	return (max(x, y) + 0.05) / (min(x, y) + 0.05)
}
