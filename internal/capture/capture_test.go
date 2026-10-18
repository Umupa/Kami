package capture

import (
	"image"
	"image/draw"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Umupa/cida-next/internal/macos"
)

// picker returns a Capturer showing a frozen "screen" rendered offscreen, at
// 2 pixels per DIP: tests never capture the live screen.
func picker(t *testing.T) (*Capturer, *ui.Tester, chan Result) {
	t.Helper()
	const w, h = 600, 200
	src := ui.Render(func(c *ui.Context) {
		ui.Column(c).Fill().Background(ui.Hex("#FFFFFF")).Children(func() {
			ui.Text(c, "Hello from the screen").FontSize(32).TextColor(ui.Hex("#000000")).Margin(60, 0, 0, 40)
		})
	}, w, h, 2)
	rgba := image.NewRGBA(src.Bounds())
	draw.Draw(rgba, rgba.Bounds(), src, src.Bounds().Min, draw.Src)
	shot, err := macos.ImageFromRGBA(rgba)
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan Result, 1)
	c := &Capturer{
		log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
		done:    func(r Result) { results <- r },
		post:    func(fn func()) { fn() },
		busy:    true,
		shot:    shot,
		bitmap:  ui.NewBitmap(rgba),
		display: mygo.Display{Bounds: mygo.Rectangle{Width: w, Height: h}},
	}
	return c, ui.NewTester(c.view, w, h), results
}

func wait(t *testing.T, results chan Result) Result {
	t.Helper()
	select {
	case r := <-results:
		return r
	case <-time.After(10 * time.Second):
		t.Fatal("no result")
	}
	return Result{}
}

func TestDragRecognizesRegion(t *testing.T) {
	c, tt, results := picker(t)
	tt.Frame()
	tt.Press(10, 10)
	tt.Move(590, 190)
	tt.Release(590, 190)
	r := wait(t, results)
	if r.Err != nil || r.Canceled {
		t.Fatalf("result %+v", r)
	}
	if !strings.Contains(r.Text, "Hello from the screen") {
		t.Errorf("recognized %q", r.Text)
	}
	if c.busy {
		t.Error("still busy after the capture")
	}
}

func TestClickCancels(t *testing.T) {
	_, tt, results := picker(t)
	tt.Frame()
	tt.Press(100, 100)
	tt.Release(101, 101)
	if r := wait(t, results); !r.Canceled {
		t.Errorf("a click gave %+v, want a cancel", r)
	}
}

func TestEscapeCancels(t *testing.T) {
	_, tt, results := picker(t)
	tt.Frame()
	tt.Key(0, ui.KeyEscape)
	if r := wait(t, results); !r.Canceled {
		t.Errorf("Escape gave %+v, want a cancel", r)
	}
}

func TestPixelRect(t *testing.T) {
	got := pixelRect(ui.Rect{X: 10, Y: 20, W: 30, H: 40}, 2)
	if got != (macos.Rect{X: 20, Y: 40, W: 60, H: 80}) {
		t.Errorf("pixelRect = %+v", got)
	}
	s := selection{x0: 50, y0: 60, x1: 10, y1: 20}
	if r := s.rect(); r != (ui.Rect{X: 10, Y: 20, W: 40, H: 40}) {
		t.Errorf("a region dragged up and left = %+v", r)
	}
}
