// Package design holds the design tokens of every window: the themes,
// Omarchy's (Tokyo Night by default), one monospaced font, flat shapes with
// hairline borders and no shadows, and the measures and motion of the chat,
// after monocode's. Views take colors from a Palette and sizes from the
// constants here, never literals of their own.
package design

import (
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

// UITheme turns a palette into the window theme, so that MyGo's own widgets
// (scroll bars, menus, text selection) take its colors and font too.
func UITheme(base ui.Theme, p *Palette, size float32) *ui.Theme {
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
	t.Font, t.FontSize = FontFamily, size
	return &t
}
