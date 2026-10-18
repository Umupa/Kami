// Package design holds the design tokens of every window, starting from
// Cida's (Design/boards/tokens.css in Xuanwo/cida). Views take colors from a
// Palette and sizes from the constants here, never literals of their own.
package design

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Palette is the set of colors of one appearance.
type Palette struct {
	Background   ui.Color // the window
	Surface      ui.Color // controls and the panel
	SurfaceDim   ui.Color // pressed and recessed areas
	SurfacePaper ui.Color // the result pane
	Border       ui.Color
	Text         ui.Color // primary text
	TextMuted    ui.Color // secondary text and labels
	TextFaint    ui.Color // tertiary text and placeholders
	TextInk      ui.Color // result text on paper
	TextControl  ui.Color // text of controls
	Hint         ui.Color // keyboard hints
	Accent       ui.Color
	AccentSoft   ui.Color // tinted fills, such as a copied button
	AccentText   ui.Color // text on Accent
	ToggleOff    ui.Color
	PanelEdge    ui.Color // the hairline around the panel
}

// Light is the light appearance.
var Light = Palette{
	Background:   ui.Hex("#FAFAF8"),
	Surface:      ui.Hex("#FFFFFF"),
	SurfaceDim:   ui.Hex("#F4F4F1"),
	SurfacePaper: ui.Hex("#F7F6F1"),
	Border:       ui.Hex("#E8E8E3"),
	Text:         ui.Hex("#1A1A18"),
	TextMuted:    ui.Hex("#8A8A83"),
	TextFaint:    ui.Hex("#B5B5AE"),
	TextInk:      ui.Hex("#161614"),
	TextControl:  ui.Hex("#4E4E49"),
	Hint:         ui.Hex("#C4C4BD"),
	Accent:       ui.Hex("#2E6B4F"),
	AccentSoft:   ui.Hex("#EAF2EE"),
	AccentText:   ui.Hex("#FFFFFF"),
	ToggleOff:    ui.Hex("#DBDBD5"),
	PanelEdge:    ui.Hex("#00000012"),
}

// Dark is the dark appearance: the same paper and ink at night.
var Dark = Palette{
	Background:   ui.Hex("#1E1E1C"),
	Surface:      ui.Hex("#262624"),
	SurfaceDim:   ui.Hex("#31312E"),
	SurfacePaper: ui.Hex("#1B1A17"),
	Border:       ui.Hex("#383835"),
	Text:         ui.Hex("#EDEDE9"),
	TextMuted:    ui.Hex("#8A8A83"),
	TextFaint:    ui.Hex("#5E5E58"),
	TextInk:      ui.Hex("#E9E6DD"),
	TextControl:  ui.Hex("#C2C2BA"),
	Hint:         ui.Hex("#4C4C47"),
	Accent:       ui.Hex("#5E9C7C"),
	AccentSoft:   ui.Hex("#233129"),
	AccentText:   ui.Hex("#FFFFFF"),
	ToggleOff:    ui.Hex("#45453F"),
	PanelEdge:    ui.Hex("#FFFFFF1F"),
}

// For returns the palette of an appearance.
func For(dark bool) *Palette {
	if dark {
		return &Dark
	}
	return &Light
}

// Type sizes in points. Text uses the system fonts (requirements Q1).
const (
	FontCaption   = 11
	FontLabel     = 11.5
	FontSecondary = 13
	FontBody      = 16
	FontResult    = 17.5
	FontResultCJK = 17

	LineHeightBody      = 1.6
	LineHeightSource    = 1.55
	LineHeightResult    = 1.65
	LineHeightResultCJK = 1.85
)

// Radii in points.
const (
	RadiusPanel   = 14
	RadiusCard    = 8
	RadiusSeg     = 7
	RadiusChip    = 6
	RadiusSegItem = 5
)

// Spacing in points.
const (
	SpaceWindowX = 28
	SpaceEntryY  = 16
	SpacePaneY   = 18
	SpaceResultY = 22
)

// The panel (requirements F1.2, F1.3).
const (
	PanelWidth       = 800
	PanelTopRatio    = 0.2 // of the visible height, where the top edge sits
	PanelMaxRatio    = 0.7 // of the visible height, the tallest the panel grows
	SourceMaxRatio   = 0.3 // of the visible height, the tallest the source grows
	ControlBarHeight = 50
	ResultFade       = 58   // points over which text fades at a scrolled edge
	ResultFadeFloor  = 0.04 // the opacity it fades to
)

// Motion (Cida's streaming-motion.md).
const (
	MotionCharIn      = 120 * time.Millisecond
	MotionIconIn      = 120 * time.Millisecond
	MotionIconSwap    = 150 * time.Millisecond
	MotionHeight      = 150 * time.Millisecond
	MotionCursorOut   = 200 * time.Millisecond
	MotionCopiedHold  = 800 * time.Millisecond
	MotionBreathe     = 1200 * time.Millisecond
	MotionCatchUp     = 400 * time.Millisecond
	MotionRateMinCPS  = 30
	MotionRateMaxCPS  = 400
	MotionRateAlpha   = 0.15
	MotionCursorWidth = 2
	MotionCursorH     = 20
)
