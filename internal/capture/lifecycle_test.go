package capture

import (
	"context"
	"errors"
	"image"
	"io"
	"log/slog"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/macos"
)

func lifecycle(t *testing.T) (*Capturer, chan func(), chan Result) {
	t.Helper()
	queue, results := make(chan func(), 2), make(chan Result, 2)
	c := &Capturer{
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		post: func(fn func()) { queue <- fn }, done: func(r Result) { results <- r },
		present: func() error { return nil }, dismiss: func() {},
		sys: backend{
			access:  func() bool { return true },
			display: func() mygo.Display { return mygo.Display{ID: 7, Bounds: mygo.Rectangle{Width: 16, Height: 12}} },
			read:    func(context.Context, *macos.Image, macos.Rect) Result { return Result{Text: "text"} },
		},
	}
	t.Cleanup(c.Close)
	return c, queue, results
}

func TestDeniedCaptureDoesNotAcquireOrPresent(t *testing.T) {
	c, _, results := lifecycle(t)
	c.sys.access = func() bool { return false }
	c.sys.warm = func() { t.Error("denied capture loaded recognition") }
	c.Start()
	if c.Busy() || !errors.Is((<-results).Err, macos.ErrNoScreenAccess) {
		t.Fatal("denied capture did not stop at the permission boundary")
	}
}

func TestStartWarmsRecognitionBeforeAcquisition(t *testing.T) {
	c, queue, results := lifecycle(t)
	warmed := 0
	c.sys.warm = func() { warmed++ }
	c.sys.freeze = func(context.Context, mygo.Display) (*macos.Image, error) {
		if warmed == 0 {
			t.Error("acquisition started before recognition warm-up")
		}
		return nil, errors.New("fixture acquisition failure")
	}
	// Vision unloads between captures, so every capture warms again.
	for i := 1; i <= 2; i++ {
		c.Start()
		(<-queue)()
		<-results
		if warmed != i {
			t.Fatalf("warm-ups: %d", warmed)
		}
	}
}

func TestAcquisitionFailureReleasesForRetry(t *testing.T) {
	c, queue, results := lifecycle(t)
	failure := errors.New("fixture acquisition failure")
	calls := 0
	c.sys.freeze = func(context.Context, mygo.Display) (*macos.Image, error) { calls++; return nil, failure }
	for range 2 {
		c.Start()
		c.Start()
		(<-queue)()
		if !errors.Is((<-results).Err, failure) || c.Busy() {
			t.Fatal("failed capture retained ownership")
		}
	}
	if calls != 2 {
		t.Fatalf("acquisitions: %d", calls)
	}
}

func TestQueuedAcquisitionCannotPresentAfterClose(t *testing.T) {
	c, queue, results := lifecycle(t)
	shot, err := macos.ImageFromRGBA(image.NewRGBA(image.Rect(0, 0, 16, 12)))
	if err != nil {
		t.Fatal(err)
	}
	c.sys.freeze = func(context.Context, mygo.Display) (*macos.Image, error) { return shot, nil }
	c.present = func() error { t.Error("late acquisition showed a window"); return nil }
	c.Start()
	apply := <-queue
	c.Close()
	apply()
	if _, err := shot.Crop(macos.Rect{W: 1, H: 1}); err == nil {
		t.Fatal("late acquisition leaked its image")
	}
	select {
	case r := <-results:
		t.Fatalf("late acquisition delivered %+v", r)
	default:
	}
}

func TestPickerFailureReleasesImageAndOwnership(t *testing.T) {
	c, queue, results := lifecycle(t)
	shot, err := macos.ImageFromRGBA(image.NewRGBA(image.Rect(0, 0, 16, 12)))
	if err != nil {
		t.Fatal(err)
	}
	c.sys.freeze = func(context.Context, mygo.Display) (*macos.Image, error) { return shot, nil }
	failure := errors.New("fixture presentation failure")
	dismissed := false
	c.dismiss = func() { dismissed = true }
	c.present = func() error { return failure }
	c.Start()
	(<-queue)()
	if !errors.Is((<-results).Err, failure) || c.Busy() {
		t.Fatal("failed picker retained ownership")
	}
	if !dismissed || c.shot != nil || c.bitmap != nil {
		t.Fatal("failed picker retained rendering resources")
	}
	if _, err := shot.Crop(macos.Rect{W: 1, H: 1}); err == nil {
		t.Fatal("failed picker leaked its image")
	}
}

func TestRepeatedCaptureReleasesPickerBeforeRecognition(t *testing.T) {
	c, queue, results := lifecycle(t)
	var shot *macos.Image
	presented, dismissed := 0, 0
	c.sys.freeze = func(context.Context, mygo.Display) (*macos.Image, error) {
		var err error
		shot, err = macos.ImageFromRGBA(image.NewRGBA(image.Rect(0, 0, 16, 12)))
		return shot, err
	}
	c.present = func() error { presented++; return nil }
	c.dismiss = func() {
		dismissed++
		if c.shot != nil || c.bitmap != nil {
			t.Error("dismiss retained capture pixels")
		}
	}
	c.sys.read = func(_ context.Context, img *macos.Image, _ macos.Rect) Result {
		if dismissed != presented {
			t.Error("recognition started before picker dismissal")
		}
		crop, err := img.Crop(macos.Rect{W: 1, H: 1})
		if err != nil {
			t.Error("recognition lost its image:", err)
		}
		crop.Release()
		return Result{Text: "text"}
	}
	for i := range 4 {
		c.Start()
		(<-queue)()
		if i%2 == 0 {
			c.finish(nil)
			if !(<-results).Canceled {
				t.Fatal("capture was not canceled")
			}
		} else {
			c.finish(&ui.Rect{W: 8, H: 8})
			(<-queue)()
			if (<-results).Text != "text" {
				t.Fatal("recognition result lost")
			}
		}
		if c.Busy() || dismissed != i+1 {
			t.Fatal("capture did not release picker and ownership")
		}
		if _, err := shot.Crop(macos.Rect{W: 1, H: 1}); err == nil {
			t.Fatal("completed capture retained image")
		}
	}
}
