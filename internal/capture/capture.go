// Package capture is screenshot recognition: the screen under the pointer is
// frozen, the user drags a region over it, and the text in that region is
// recognized on this Mac. The image never leaves the Mac and is not kept.
package capture

import (
	"log/slog"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Umupa/cida-next/internal/macos"
)

// Result is what a capture found.
type Result struct {
	// Text is the recognized lines, in reading order, joined by newlines.
	Text string
	// Canceled reports that the user pressed Escape or dragged no region.
	Canceled bool
	// Err is a capture or recognition failure.
	Err error
}

// Capturer runs captures one at a time. Start runs on the main thread.
type Capturer struct {
	log *slog.Logger
	// done hears every capture's result on the main thread.
	done func(Result)
	// post runs a function on the main thread: mygo.RunOnMain, or a test's.
	post func(func())

	win     *mygo.Window
	display mygo.Display
	shot    *macos.Image
	bitmap  *ui.Bitmap
	sel     selection
	busy    bool
}

// New returns a Capturer, and warms Vision up in the background.
func New(log *slog.Logger, done func(Result)) *Capturer {
	go macos.WarmUpVision()
	return &Capturer{log: log, done: done, post: mygo.RunOnMain}
}

const captureTimeout = 3 * time.Second

// Start freezes the display under the pointer and shows the region picker.
// Without Screen Recording it asks for it and reports the error.
func (c *Capturer) Start() {
	if c.busy {
		return
	}
	if !macos.ScreenAccess() {
		macos.RequestScreenAccess()
		c.done(Result{Err: macos.ErrNoScreenAccess})
		return
	}
	c.busy = true
	d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	go func() {
		start := time.Now()
		b := d.Bounds
		shot, err := macos.CaptureRect(macos.Rect{X: float64(b.X), Y: float64(b.Y), W: float64(b.Width), H: float64(b.Height)}, captureTimeout)
		var bitmap *ui.Bitmap
		if err == nil {
			bitmap = ui.NewBitmap(shot.RGBA())
		}
		c.log.Info("capture frozen", "ms", time.Since(start).Milliseconds(), "err", err)
		c.post(func() {
			if err != nil {
				c.busy = false
				c.done(Result{Err: err})
				return
			}
			c.display, c.shot, c.bitmap, c.sel = d, shot, bitmap, selection{}
			c.show()
		})
	}()
}

// show covers the display with the frozen picture.
func (c *Capturer) show() {
	b := c.display.Bounds
	if c.win == nil {
		c.win = mygo.NewWindow(mygo.WindowOptions{
			Title:             "cida-next 截图",
			Panel:             true,
			Hidden:            true,
			Frameless:         true,
			AlwaysOnTop:       true,
			DisableResize:     true,
			DisableMinimize:   true,
			DisableMaximize:   true,
			DisableFullScreen: true,
			DisableShadow:     true,
			Content:           ui.View(c.view),
		})
		// Escape closes a panel; it cancels the capture instead.
		c.win.OnClose(func(e *mygo.CloseEvent) {
			e.PreventDefault()
			c.finish(nil)
		})
		c.win.OnBlur(func() { c.finish(nil) })
	}
	c.win.SetBounds(b)
	c.win.Show()
	c.win.Invalidate()
}

// finish hides the picker and recognizes the region, or cancels without one.
func (c *Capturer) finish(region *ui.Rect) {
	if !c.busy {
		return
	}
	c.busy = false
	if c.win != nil {
		c.win.Hide()
	}
	shot, display := c.shot, c.display
	c.shot, c.bitmap = nil, nil
	if region == nil {
		shot.Release()
		c.done(Result{Canceled: true})
		return
	}
	px := pixelRect(*region, float64(shot.W)/float64(display.Bounds.Width))
	go func() {
		defer shot.Release()
		start := time.Now()
		res := recognize(shot, px)
		c.log.Info("capture recognized", "ms", time.Since(start).Milliseconds(), "found", res.Text != "", "err", res.Err)
		c.post(func() { c.done(res) })
	}()
}

func recognize(shot *macos.Image, px macos.Rect) Result {
	crop, err := shot.Crop(px)
	if err != nil {
		return Result{Err: err}
	}
	defer crop.Release()
	lines, err := macos.RecognizeText(crop)
	if err != nil {
		return Result{Err: err}
	}
	texts := make([]string, 0, len(lines))
	for _, l := range lines {
		if t := strings.TrimSpace(l.Text); t != "" {
			texts = append(texts, t)
		}
	}
	return Result{Text: strings.Join(texts, "\n")}
}
