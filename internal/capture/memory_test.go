package capture

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo"

	"github.com/Umupa/Kami/internal/macos"
)

type manualMemoryTimer struct{ stopped bool }

func (t *manualMemoryTimer) Stop() bool { t.stopped = true; return true }

func TestIdleMemoryDebouncesAndRejectsLateCallbacks(t *testing.T) {
	var callbacks []func()
	var timers []*manualMemoryTimer
	releases := 0
	m := &idleMemory{
		after: func(d time.Duration, fn func()) memoryTimer {
			if d != memoryIdleDelay {
				t.Fatalf("delay: %s", d)
			}
			timer := &manualMemoryTimer{}
			timers = append(timers, timer)
			callbacks = append(callbacks, fn)
			return timer
		},
		release: func() { releases++ },
	}
	m.schedule()
	m.cancel()
	callbacks[0]() // A stopped timer may already have queued its callback.
	if releases != 0 || !timers[0].stopped {
		t.Fatal("new capture did not cancel pending release")
	}
	m.schedule()
	m.schedule()
	callbacks[1]()
	callbacks[2]()
	if releases != 1 || !timers[1].stopped {
		t.Fatal("burst was not coalesced")
	}
	m.schedule()
	m.close()
	callbacks[3]()
	m.schedule()
	if releases != 1 || len(callbacks) != 4 || !timers[3].stopped {
		t.Fatal("closed capturer scheduled a release")
	}
}

func TestCaptureMemoryScheduling(t *testing.T) {
	c, _, results := lifecycle(t)
	scheduled := 0
	c.memory = &idleMemory{
		after:   func(time.Duration, func()) memoryTimer { scheduled++; return &manualMemoryTimer{} },
		release: func() { t.Error("unexpected real release") },
	}
	c.complete(Result{})
	<-results
	if scheduled != 0 {
		t.Fatal("capture without pixels scheduled release")
	}
	c.memoryDirty = true
	c.complete(Result{})
	<-results
	c.complete(Result{})
	<-results
	if scheduled != 1 || c.memoryDirty {
		t.Fatal("completed pixels must schedule exactly one release")
	}
}

func TestFailedRetryPreservesPendingMemoryRelease(t *testing.T) {
	c, queue, results := lifecycle(t)
	scheduled := 0
	timer := &manualMemoryTimer{}
	c.memory = &idleMemory{
		after:   func(time.Duration, func()) memoryTimer { scheduled++; return timer },
		release: func() { t.Error("unexpected release") },
	}
	c.memory.schedule()
	c.sys.freeze = func(context.Context, mygo.Display) (*macos.Image, error) { return nil, errors.New("capture failed") }
	c.Start()
	if !timer.stopped {
		t.Fatal("new capture left release pending")
	}
	(<-queue)()
	if (<-results).Err == nil || scheduled != 2 {
		t.Fatal("failed retry lost previous release")
	}
}
