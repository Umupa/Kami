// Package design holds the design tokens of every window: the themes, after
// Omarchy's (Tokyo Night by default), one monospaced font, flat shapes with
// hairline borders and no shadows, and the measures and motion of the chat,
// after monocode's. Views take colors from a Palette and sizes from the
// constants here, never literals of their own.
package design

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
)

// Palette is the set of colors of one theme.
type Palette struct {
	Dark bool

	Background ui.Color // the panel
	Surface    ui.Color // code blocks, table headers, chips
	Raised     ui.Color // the chosen row, the active tab
	Border     ui.Color // hairlines between parts, around blocks
	Edge       ui.Color // the panel's frame, tinted by the accent

	Text   ui.Color // body text
	Bright ui.Color // strong text, the source
	Muted  ui.Color // meta lines, descriptions
	Faint  ui.Color // placeholders, key hints

	Accent     ui.Color // the run key, headings, the active mode
	AccentText ui.Color // text on Accent
	Link       ui.Color
	Code       ui.Color // code spans
	Quote      ui.Color // the bar along quotes

	// Operator colors operator mode: its command, its pill and its sparkles.
	Operator     ui.Color
	OperatorFill ui.Color
	Sparkle      ui.Color
	Danger       ui.Color

	// Syntax colors code by kind of token.
	Keyword, String, Number, Comment, Function, Type ui.Color
}

// TokyoNight is the dark theme, Omarchy's default.
var TokyoNight = Palette{
	Dark:         true,
	Background:   ui.Hex("#1A1B26"),
	Surface:      ui.Hex("#16161E"),
	Raised:       ui.Hex("#292E42"),
	Border:       ui.Hex("#292E42"),
	Edge:         ui.Hex("#7AA2F759"),
	Text:         ui.Hex("#A9B1D6"),
	Bright:       ui.Hex("#C0CAF5"),
	Muted:        ui.Hex("#737AA2"),
	Faint:        ui.Hex("#565F89"),
	Accent:       ui.Hex("#7AA2F7"),
	AccentText:   ui.Hex("#1A1B26"),
	Link:         ui.Hex("#7DCFFF"),
	Code:         ui.Hex("#9ECE6A"),
	Quote:        ui.Hex("#BB9AF7"),
	Operator:     ui.Hex("#7DCFFF"),
	OperatorFill: ui.Hex("#7DCFFF1F"),
	Sparkle:      ui.Hex("#E0AF68"),
	Danger:       ui.Hex("#F7768E"),
	Keyword:      ui.Hex("#BB9AF7"),
	String:       ui.Hex("#9ECE6A"),
	Number:       ui.Hex("#FF9E64"),
	Comment:      ui.Hex("#565F89"),
	Function:     ui.Hex("#7AA2F7"),
	Type:         ui.Hex("#2AC3DE"),
}

// TokyoNightDay is the light theme.
var TokyoNightDay = Palette{
	Background:   ui.Hex("#E1E2E7"),
	Surface:      ui.Hex("#D5D6DB"),
	Raised:       ui.Hex("#C4C8DA"),
	Border:       ui.Hex("#C4C8DA"),
	Edge:         ui.Hex("#2E7DE959"),
	Text:         ui.Hex("#3760BF"),
	Bright:       ui.Hex("#343B58"),
	Muted:        ui.Hex("#6172B0"),
	Faint:        ui.Hex("#848CB5"),
	Accent:       ui.Hex("#2E7DE9"),
	AccentText:   ui.Hex("#E1E2E7"),
	Link:         ui.Hex("#007197"),
	Code:         ui.Hex("#587539"),
	Quote:        ui.Hex("#9854F1"),
	Operator:     ui.Hex("#007197"),
	OperatorFill: ui.Hex("#0071971F"),
	Sparkle:      ui.Hex("#8C6C3E"),
	Danger:       ui.Hex("#F52A65"),
	Keyword:      ui.Hex("#9854F1"),
	String:       ui.Hex("#587539"),
	Number:       ui.Hex("#B15C00"),
	Comment:      ui.Hex("#848CB5"),
	Function:     ui.Hex("#2E7DE9"),
	Type:         ui.Hex("#007197"),
}

// Themes are the themes config.json names, besides "auto", which follows
// the system's appearance.
var Themes = map[string]*Palette{
	"tokyo-night":     &TokyoNight,
	"tokyo-night-day": &TokyoNightDay,
}

// For returns the palette of a theme name; "auto" or an unknown name follows
// the appearance.
func For(theme string, dark bool) *Palette {
	if p, ok := Themes[theme]; ok {
		return p
	}
	if dark {
		return &TokyoNight
	}
	return &TokyoNightDay
}

// Font is the one family of every window: Omarchy's, falling back to the
// system's monospaced font where it is not installed.
const (
	Font         = "CaskaydiaCove Nerd Font"
	FallbackFont = "SF Mono"
)

// Type sizes in points, and line heights as multiples of the size.
const (
	FontCaption = 11
	FontLabel   = 12
	FontBody    = 13.5
	FontSource  = 15

	LineHeightBody   = 1.65
	LineHeightSource = 1.5
)

// Radii in points: flat, barely rounded.
const (
	RadiusPanel = 8
	RadiusBlock = 4
	RadiusChip  = 3
)

// Spacing in points.
const (
	SpaceX       = 22
	SpaceSourceY = 16
	SpaceResultY = 18
	SpaceTurns   = 22
)

// The panel (requirements F1.2, F1.3).
const (
	PanelWidth       = 800
	PanelTopRatio    = 0.2 // of the visible height, where the top edge sits
	PanelMaxRatio    = 0.7 // of the visible height, the tallest the panel grows
	SourceMaxRatio   = 0.3 // of the visible height, the tallest the source grows
	ControlBarHeight = 44
)

// Motion, after monocode's.
const (
	MotionWordFade   = 320 * time.Millisecond // each word as it streams in
	MotionCatchUp    = 400 * time.Millisecond
	MotionHeight     = 150 * time.Millisecond
	MotionCopiedHold = 1500 * time.Millisecond
	MotionThinking   = 1600 * time.Millisecond // the pulse while waiting
	// Operator mode: the sheen across its name as it turns on, and the
	// sparkles of a request sent in it.
	MotionSheen    = 900 * time.Millisecond
	MotionSparkles = 3200 * time.Millisecond
)

// MonoFont returns Font when it is installed, else FallbackFont: Core Text
// would otherwise fall back to a proportional font.
func MonoFont() string {
	monoOnce.Do(func() {
		monoFont = FallbackFont
		home, _ := os.UserHomeDir()
		for _, dir := range []string{filepath.Join(home, "Library", "Fonts"), "/Library/Fonts"} {
			if m, _ := filepath.Glob(filepath.Join(dir, "CaskaydiaCove*NerdFont*-Regular.ttf")); len(m) > 0 {
				monoFont = Font
				return
			}
		}
	})
	return monoFont
}

var (
	monoOnce sync.Once
	monoFont string
)

// UITheme turns a palette into the window theme, so that MyGo's own widgets
// (scroll bars, menus, text selection) take its colors and font too.
func UITheme(base ui.Theme, p *Palette, font string) *ui.Theme {
	t := base
	t.Dark = p.Dark
	t.Background, t.Surface = p.Background, p.Surface
	t.SurfaceHover, t.SurfacePressed = p.Raised, p.Raised
	t.Border, t.Text, t.TextMuted = p.Border, p.Text, p.Faint
	t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = p.Accent, p.Accent, p.Accent, p.AccentText
	t.Danger = p.Danger
	t.Selection = p.Accent.Alpha(0.3)
	t.Focus = p.Accent
	t.Scrollbar = p.Faint.Alpha(0.5)
	t.Radius = RadiusChip
	t.Font, t.FontSize = font, FontBody
	return &t
}
