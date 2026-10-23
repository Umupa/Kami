package capture

import (
	"image"
	"math"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/design"
	"github.com/Umupa/Kami/internal/macos"
)

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

const minRegion = 4
const captureHint = "拖动框选文字 · Esc 取消"

func (c *Capturer) view(ctx *ui.Context) {
	root := ctx.Root().Background(ui.Transparent)
	if ctx.Shortcut(0, ui.KeyEscape) {
		c.finish(nil)
		return
	}
	w, h := ctx.Size()
	key := [2]uint64{c.generation, 0}
	if !c.fadeStarted {
		root.Animate(key, 0, 0)
		c.fadeStarted = true
	}
	veilAlpha := root.AnimateWith(key, 1, design.CaptureFade, ui.CubicBezier(0.33, 1, 0.68, 1))
	bitmap, sel := c.bitmap, c.sel
	s := sel.rect()
	veil, edge := design.CapturePaperVeil, design.CapturePaperEdge
	if c.ink {
		veil, edge = design.CaptureInkVeil, design.CaptureInkEdge
	}
	ui.Box(ctx).Fill().Cursor(ui.CursorCrosshair).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Image(bitmap, r, ui.FillBox)
		p.Fill(r, veil.Alpha(veilAlpha), 0)
		if !sel.dragging || s.W < 1 || s.H < 1 {
			return
		}
		if !c.ink {
			p.Shadow(s, design.CaptureRadius, 0, 2, 6, 0, design.CaptureContact)
			p.Shadow(s, design.CaptureRadius, 0, 28, 72, 0, design.CaptureAmbient)
		}
		p.Clip(s, design.CaptureRadius, func() { p.Image(bitmap, r, ui.FillBox) })
		p.Stroke(s, edge, design.CaptureRadius, 1)
	}).HandleInput(func(ev ui.InputEvent) bool {
		x, y := max(0, min(w, ev.X)), max(0, min(h, ev.Y))
		switch ev.Kind {
		case ui.InputPointerDown:
			if ev.Button != 0 {
				c.finish(nil)
				return true
			}
			c.sel = selection{dragging: true, x0: x, y0: y, x1: x, y1: y}
		case ui.InputPointerMove:
			if c.sel.dragging {
				c.sel.x1, c.sel.y1 = x, y
			}
		case ui.InputPointerUp:
			// MyGo synthesizes a release on surface blur. Losing key status
			// is not the user's confirmation and must not start recognition.
			if c.keyWindow != nil && !c.keyWindow() {
				c.sel = selection{}
				return true
			}
			if !c.sel.dragging {
				return true
			}
			c.sel.x1, c.sel.y1 = x, y
			r := c.sel.rect()
			if r.W < minRegion || r.H < minRegion {
				c.finish(nil)
			} else {
				c.finish(&r)
			}
		default:
			return false
		}
		return true
	})
	// The label never consumes a press; its entire area belongs to the canvas.
	fill, text, border, contact, ambient := design.CaptureHint(ctx.Theme().Dark)
	label := "Kami  │  " + captureHint
	tw, th := ctx.MeasureText(0, ui.Span{Text: label, Size: design.CaptureHintSize})
	hw, hh := tw+32, max(float32(design.CaptureHintHeight), th+16)
	y := float32(c.display.WorkArea.Y-c.display.Bounds.Y) + float32(math.Floor(float64(c.display.WorkArea.Height)*design.PanelTopRatio))
	if c.display.WorkArea.Height == 0 {
		y = h * design.PanelTopRatio
	}
	c.hint = ui.Rect{X: (w - hw) / 2, Y: y, W: hw, H: hh}
	target := float32(1)
	if sel.dragging && intersects(s, c.hint) {
		target = 0
	}
	hintAlpha := root.AnimateWith([2]uint64{c.generation, 1}, target, design.CaptureFade, ui.CubicBezier(0.33, 1, 0.68, 1))
	ui.Box(ctx).Label(captureHint).PassThrough().Absolute().Left(c.hint.X).Top(y).Size(hw, hh).Opacity(veilAlpha * hintAlpha).Draw(func(p *ui.Painter, r ui.Rect) {
		p.Shadow(r, hh/2, 0, 2, 6, 0, contact)
		p.Shadow(r, hh/2, 0, 28, 72, 0, ambient)
		p.Fill(r, fill, hh/2)
		p.Stroke(r, border, hh/2, 1)
		p.Text(r.X+16, r.Y+(r.H-th)/2, label, design.CaptureHintSize, text)
	})
}

func intersects(a, b ui.Rect) bool {
	return a.X < b.X+b.W && a.X+a.W > b.X && a.Y < b.Y+b.H && a.Y+a.H > b.Y
}

// pixelRect uses both image axes and rounds outwards, preserving edge pixels.
func pixelRect(r ui.Rect, w, h float64, imageW, imageH int) macos.Rect {
	if w <= 0 || h <= 0 {
		return macos.Rect{}
	}
	sx, sy := float64(imageW)/w, float64(imageH)/h
	x0, y0 := max(0, math.Floor(float64(r.X)*sx)), max(0, math.Floor(float64(r.Y)*sy))
	x1, y1 := min(float64(imageW), math.Ceil(float64(r.X+r.W)*sx)), min(float64(imageH), math.Ceil(float64(r.Y+r.H)*sy))
	return macos.Rect{X: x0, Y: y0, W: max(0, x1-x0), H: max(0, y1-y0)}
}

// darkScreen measures the frozen image, independently of the hint appearance.
func darkScreen(img *image.RGBA) bool {
	var luminance float64
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			p := img.RGBAAt(img.Rect.Min.X+x*img.Rect.Dx()/32, img.Rect.Min.Y+y*img.Rect.Dy()/32)
			luminance += 0.2126*float64(p.R) + 0.7152*float64(p.G) + 0.0722*float64(p.B)
		}
	}
	return luminance/1024/255 < 0.5
}
