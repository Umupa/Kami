package panel

import (
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/design"
)

// splitDrag is a drag of the divider between the source and the result, as
// a terminal's split pane: the source follows the pointer.
type splitDrag struct {
	active        bool
	startY, start float32 // the pointer's y in the window, the source's height
}

// minResult is the room the source's divider leaves for the reply.
const minResult = 120

// panelBorder is the panel's border, inside the window's height.
const panelBorder = 1

// clampSource keeps a height the user dragged the source to within the
// panel: what it holds, less what keeps its own height under the source and
// room for a reply, so the divider and the bars stay in view as the window
// is resized shorter, a reply comes or the font grows; dragging back finds
// the divider. It is the tallest the source may be, down to a caret line.
func (p *Panel) clampSource(h float32) float32 {
	if p.underSource > 0 {
		room := float32(p.place.maxHeight)
		if p.fixedLayout() {
			room = float32(p.fixedHeight) - 2*panelBorder
		}
		room -= p.underSource
		if p.sess != nil {
			room -= minResult
		}
		h = min(h, room)
	}
	return max(h, p.minSourceHeight())
}

// minSourceHeight leaves room for a whole caret line inside the padding;
// a shorter viewport makes the text area alternate between revealing its
// top and bottom on each edit. Round up to avoid fractional layout loss.
func (p *Panel) minSourceHeight() float32 {
	return float32(math.Ceil(float64(p.sz.Source*design.LineHeightSource + 2*design.SpaceSourceY)))
}

// splitter is the divider under the source: drag it to share the panel's
// height between the source and the result, double-click to go back to the
// default.
func (p *Panel) splitter(c *ui.Context, pal *design.Palette) {
	handle := ui.Box(c.Key("splitter")).Bind(&p.splitHandle).Height(7).Shrink(0).
		Cursor(ui.CursorResizeNS).Label("调整原文与结果的高度")
	line := pal.Border
	if p.split.active || handle.Hovered() {
		line = pal.Accent
	}
	handle.Draw(func(pt *ui.Painter, r ui.Rect) {
		pt.Fill(ui.Rect{X: r.X, Y: r.Y + r.H/2 - 0.5, W: r.W, H: 1}, line, 0)
		if line == pal.Accent {
			// A grip in the middle, as terminal splits show.
			pt.Fill(ui.Rect{X: r.X + r.W/2 - 14, Y: r.Y + r.H/2 - 1.5, W: 28, H: 3}, line, 1.5)
		}
	})
	// The handle keeps the pointer it was pressed with, and moves as the
	// source grows: the pointer's place in the window is the handle's box
	// as laid out now, read through its handle, plus the event's y in it.
	handle.HandleInput(func(ev ui.InputEvent) bool {
		y := p.splitHandle.Bounds(c).Y + ev.Y
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Clicks >= 2 {
				p.sourceHeight, p.split.active = 0, false
				return true
			}
			p.split = splitDrag{active: true, startY: y, start: p.sourceIn.Bounds(c).H}
			return true
		case ui.InputPointerMove:
			if !p.split.active {
				return false
			}
			p.sourceHeight = p.clampSource(p.split.start + y - p.split.startY)
			return true
		case ui.InputPointerUp:
			p.split.active = false
			return true
		}
		return false
	})
}
