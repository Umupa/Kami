package capture

import (
	"context"
	"errors"
	"image"
	"io"
	"log/slog"
	"testing"

	"github.com/egoist/mygo"

	"github.com/Umupa/Gloss/internal/macos"
)

func lifecycle(t *testing.T) (*Capturer, chan func(), chan Result) {
	t.Helper()
	queue, results := make(chan func(), 2), make(chan Result, 2)
	c := &Capturer{
		log:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		post: func(fn func()) { queue <- fn }, done: func(r Result) { results <- r },
		present: func() error { return nil }, dismiss: func() {},
		sys: backend{
			access: func() bool { return true }, request: func() bool { t.Error("unexpected permission request"); return false },
			display: func() mygo.Display { return mygo.Display{ID: 7, Bounds: mygo.Rectangle{Width: 16, Height: 12}} },
			read:    func(context.Context, *macos.Image, macos.Rect) Result { return Result{Text: "text"} },
		},
	}
	t.Cleanup(c.Close)
	return c, queue, results
}

func TestDeniedCaptureDoesNotAcquireOrPresent(t *testing.T) {
	c, _, results := lifecycle(t)
	asked := false
	c.sys.access = func() bool { return false }
	c.sys.request = func() bool { asked = true; return false }
	c.Start()
	if !asked || c.Busy() || !errors.Is((<-results).Err, macos.ErrNoScreenAccess) {
		t.Fatal("denied capture did not stop at the permission boundary")
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
	c.present = func() error { return failure }
	c.Start()
	(<-queue)()
	if !errors.Is((<-results).Err, failure) || c.Busy() {
		t.Fatal("failed picker retained ownership")
	}
	if _, err := shot.Crop(macos.Rect{W: 1, H: 1}); err == nil {
		t.Fatal("failed picker leaked its image")
	}
}
