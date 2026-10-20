package capture

import (
	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Gloss/internal/macos"
)

// selection is the region being dragged, in DIPs of the window.
type selection struct {
	dragging       bool
	x0, y0, x1, y1 float32
}

func (s selection) rect() ui.Rect {
	return ui.Rect{X: min(s.x0, s.x1), Y: min(s.y0, s.y1), W: abs(s.x1 - s.x0), H: abs(s.y1 - s.y0)}
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// minRegion is the smallest region worth recognizing; a click is a cancel.
const minRegion = 4

var (
	fog    = ui.RGBA(0, 0, 0, 0.35)
	edge   = ui.RGBA(255, 255, 255, 0.9)
	shadow = ui.RGBA(0, 0, 0, 0.5)
)

func (c *Capturer) view(ctx *ui.Context) {
	ctx.Root().Background(ui.Transparent)
	if ctx.Shortcut(0, ui.KeyEscape) {
		c.finish(nil)
		return
	}
	bitmap, sel := c.bitmap, c.sel
	ui.Box(ctx).Fill().Cursor(ui.CursorCrosshair).Draw(func(p *ui.Painter, r ui.Rect) {
		if bitmap != nil {
			p.Image(bitmap, r, ui.FillBox)
		}
		s := sel.rect()
		if !sel.dragging || s.W < 1 || s.H < 1 {
			p.Fill(r, fog, 0)
			return
		}
		// Fog everywhere but the region.
		p.Fill(ui.Rect{X: r.X, Y: r.Y, W: r.W, H: s.Y - r.Y}, fog, 0)
		p.Fill(ui.Rect{X: r.X, Y: s.Y + s.H, W: r.W, H: r.Y + r.H - s.Y - s.H}, fog, 0)
		p.Fill(ui.Rect{X: r.X, Y: s.Y, W: s.X - r.X, H: s.H}, fog, 0)
		p.Fill(ui.Rect{X: s.X + s.W, Y: s.Y, W: r.X + r.W - s.X - s.W, H: s.H}, fog, 0)
		p.Stroke(ui.Rect{X: s.X - 1, Y: s.Y - 1, W: s.W + 2, H: s.H + 2}, shadow, 0, 1)
		p.Stroke(s, edge, 0, 1)
	}).HandleInput(func(ev ui.InputEvent) bool {
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 {
				c.finish(nil)
				return true
			}
			c.sel = selection{dragging: true, x0: ev.X, y0: ev.Y, x1: ev.X, y1: ev.Y}
			return true
		case ui.InputPointerMove:
			if c.sel.dragging {
				c.sel.x1, c.sel.y1 = ev.X, ev.Y
			}
			return true
		case ui.InputPointerUp:
			if !c.sel.dragging {
				return true
			}
			c.sel.x1, c.sel.y1 = ev.X, ev.Y
			r := c.sel.rect()
			if r.W < minRegion || r.H < minRegion {
				c.finish(nil)
				return true
			}
			c.finish(&r)
			return true
		}
		return false
	})
}

// pixelRect turns a region in DIPs into pixels of a capture taken at scale
// pixels per DIP.
func pixelRect(r ui.Rect, scale float64) macos.Rect {
	return macos.Rect{
		X: float64(r.X) * scale,
		Y: float64(r.Y) * scale,
		W: float64(r.W) * scale,
		H: float64(r.H) * scale,
	}
}
