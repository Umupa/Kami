//go:build soak

package capture

import (
	"context"
	"image"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/Umupa/Kami/internal/macos"
	"github.com/Umupa/Kami/internal/soak"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func TestSoakCapture4K(t *testing.T) {
	soakCapture4K(t, 1)
}

func TestSoakCapture4KReuse(t *testing.T) {
	if os.Getenv("KAMI_SOAK_IDLE") == "" {
		t.Skip("set KAMI_SOAK_IDLE to observe recovery between batches")
	}
	soakCapture4K(t, 3)
}

func soakCapture4K(t *testing.T, batches int) {
	t.Helper()
	c, queue, results := lifecycle(t)
	c.memory = newIdleMemory()
	c.sys.display = func() mygo.Display { return mygo.Display{Bounds: mygo.Rectangle{Width: 3840, Height: 2160}} }
	c.sys.freeze = func(context.Context, mygo.Display) (*macos.Image, error) {
		return macos.ImageFromRGBA(image.NewRGBA(image.Rect(0, 0, 3840, 2160)))
	}
	rounds := soak.Rounds(t, 100)
	var recovered soak.Sample
	for batch := 1; batch <= batches; batch++ {
		t.Logf("batch=%d/%d", batch, batches)
		var baseline, final soak.Sample
		var total, first time.Duration
		for i := 1; i <= rounds; i++ {
			start := time.Now()
			c.Start()
			(<-queue)()
			if c.bitmap == nil {
				t.Fatal("no capture bitmap")
			}
			if i%2 == 0 {
				c.finish(nil)
				if !(<-results).Canceled {
					t.Fatal("capture did not cancel")
				}
			} else {
				c.finish(&ui.Rect{W: 100, H: 100})
				(<-queue)()
				if (<-results).Text != "text" {
					t.Fatal("capture lost its result")
				}
			}
			if c.Busy() || c.shot != nil || c.bitmap != nil {
				t.Fatal("capture retained resources")
			}
			elapsed := time.Since(start)
			total += elapsed
			if i == 1 {
				first = elapsed
			}
			if i%max(1, rounds/5) == 0 || i == rounds {
				final = soak.Measure(t, i)
				if i == rounds/5 {
					baseline = final
				}
			}
		}
		soak.Check(t, baseline, final)
		t.Logf("batch=%d first=%s mean=%s", batch, first, total/time.Duration(rounds))
		soak.Idle(t)
		if batches > 1 {
			sample := soak.Measure(t, batch)
			if batch == 1 {
				recovered = sample
			} else {
				soak.Check(t, recovered, sample)
			}
		}
	}
	runtime.KeepAlive(c)
}
