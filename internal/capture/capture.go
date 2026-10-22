// Package capture freezes the display under the pointer, lets the user frame
// text, and recognizes it locally. Images are neither saved nor uploaded.
package capture

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/macos"
)

// Result is what a capture found.
type Result struct {
	Text     string
	Canceled bool
	Err      error
}
type phase uint8

const (
	idle phase = iota
	freezing
	selecting
	recognizing
	closed
)
const captureTimeout = 10 * time.Second
const recognitionTimeout = 30 * time.Second

// backend separates system acquisition from the picker and its lifecycle.
type backend struct {
	access  func() bool
	request func() bool
	display func() mygo.Display
	freeze  func(context.Context, mygo.Display) (*macos.Image, error)
	read    func(context.Context, *macos.Image, macos.Rect) Result
}

// Capturer owns one complete capture, including recognition. Its methods run
// on the main thread; workers return through post and cannot change its state.
type Capturer struct {
	log         *slog.Logger
	done        func(Result)
	post        func(func())
	sys         backend
	present     func() error
	dismiss     func()
	keyWindow   func() bool
	state       phase
	generation  uint64
	cancel      context.CancelFunc
	win         *mygo.Window
	display     mygo.Display
	shot        *macos.Image
	bitmap      *ui.Bitmap
	sel         selection
	ink         bool
	fadeStarted bool
	hint        ui.Rect
}

// New returns a Capturer and warms both recognition languages in the background.
func New(log *slog.Logger, done func(Result)) *Capturer {
	c := &Capturer{log: log, done: done, post: mygo.RunOnMain, sys: backend{
		access: macos.ScreenAccess, request: macos.RequestScreenAccess,
		display: func() mygo.Display { return mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint()) },
		freeze: func(ctx context.Context, d mygo.Display) (*macos.Image, error) {
			return macos.CaptureDisplay(ctx, uint32(d.ID), int(math.Round(float64(d.Bounds.Width)*d.ScaleFactor)), int(math.Round(float64(d.Bounds.Height)*d.ScaleFactor)))
		},
		read: recognize,
	}}
	c.present, c.dismiss = c.show, func() {
		if c.win != nil {
			c.win.Hide()
		}
	}
	c.keyWindow = func() bool { return c.win != nil && c.win.IsFocused() }
	go macos.WarmUpVision()
	return c
}

// Busy includes acquisition, framing, recognition and shutdown.
func (c *Capturer) Busy() bool { return c.state != idle }

// Start freezes the display under the pointer. Repeated entry points wait for
// the result instead of hiding the picker or superseding recognition.
func (c *Capturer) Start() {
	if c.Busy() {
		return
	}
	if !c.sys.access() {
		c.sys.request()
		c.done(Result{Err: macos.ErrNoScreenAccess})
		return
	}
	c.state = freezing
	c.generation++
	id := c.generation
	d := c.sys.display()
	ctx, cancel := context.WithTimeout(context.Background(), captureTimeout)
	c.cancel = cancel
	go func() {
		defer cancel()
		start := time.Now()
		shot, err := c.sys.freeze(ctx, d)
		var bitmap *ui.Bitmap
		var ink bool
		if err == nil {
			rgba := shot.RGBA()
			bitmap, ink = ui.NewBitmap(rgba), darkScreen(rgba)
		}
		c.log.Info("capture frozen", "ms", time.Since(start).Milliseconds(), "err", err)
		c.post(func() {
			if c.generation != id || c.state != freezing {
				shot.Release()
				return
			}
			c.cancel = nil
			if err != nil {
				shot.Release()
				c.complete(Result{Err: err})
				return
			}
			c.display, c.shot, c.bitmap, c.sel = d, shot, bitmap, selection{}
			c.ink, c.fadeStarted, c.hint = ink, false, ui.Rect{}
			c.state = selecting
			if err := c.present(); err != nil {
				c.shot.Release()
				c.shot, c.bitmap = nil, nil
				c.complete(Result{Err: err})
			}
		})
	}()
}

func (c *Capturer) show() error {
	if c.win == nil {
		c.win = mygo.NewWindow(mygo.WindowOptions{
			Title: "Kami 截图", Panel: true, Hidden: true, Frameless: true,
			AlwaysOnTop: true, DisableResize: true, DisableMinimize: true,
			DisableMaximize: true, DisableFullScreen: true, DisableShadow: true,
			Content: ui.View(c.view),
		})
		c.win.OnClose(func(e *mygo.CloseEvent) {
			if c.state == closed {
				return
			}
			e.PreventDefault()
			c.finish(nil)
		})
	}
	// Framing is canceled only by its explicit controls. Transient key status
	// changes must not discard the frozen image.
	if err := macos.ConfigureCaptureWindow(c.win.NativeHandle()); err != nil {
		return err
	}
	c.win.SetBounds(c.display.Bounds)
	c.win.Show()
	c.win.Invalidate()
	c.log.Info("capture picker shown", "display", c.display.ID, "want", c.display.Bounds, "got", c.win.Bounds())
	return nil
}

// finish hides the picker once; recognition still owns the capture afterwards.
func (c *Capturer) finish(region *ui.Rect) {
	if c.state != selecting {
		return
	}
	shot := c.shot
	c.shot, c.bitmap = nil, nil
	if region == nil {
		c.state = idle
		c.dismiss()
		shot.Release()
		c.log.Info("capture canceled")
		c.done(Result{Canceled: true})
		return
	}
	c.state = recognizing
	c.dismiss()
	px := pixelRect(*region, float64(c.display.Bounds.Width), float64(c.display.Bounds.Height), shot.W, shot.H)
	id := c.generation
	ctx, cancel := context.WithTimeout(context.Background(), recognitionTimeout)
	c.cancel = cancel
	go func() {
		defer cancel()
		defer shot.Release()
		start := time.Now()
		res := c.sys.read(ctx, shot, px)
		c.log.Info("capture recognized", "ms", time.Since(start).Milliseconds(), "found", res.Text != "", "err", res.Err)
		c.post(func() {
			if c.generation != id || c.state != recognizing {
				return
			}
			c.complete(res)
		})
	}()
}

func (c *Capturer) complete(r Result) {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.state = idle
	c.done(r)
}

// Close cancels workers and invalidates queued results before the app quits.
func (c *Capturer) Close() {
	c.generation++
	c.state = closed
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	c.shot.Release()
	c.shot, c.bitmap = nil, nil
	c.dismiss()
}

func recognize(ctx context.Context, shot *macos.Image, px macos.Rect) Result {
	crop, err := shot.Crop(px)
	if err != nil {
		return Result{Err: err}
	}
	defer crop.Release()
	lines, err := macos.RecognizeTextContext(ctx, crop)
	if err != nil {
		return Result{Err: err}
	}
	return Result{Text: recognizedText(lines)}
}
