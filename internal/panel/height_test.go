package panel

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestHeightSetterAppliesLastInOrder(t *testing.T) {
	var (
		mu      sync.Mutex
		applied []int
		busy    atomic.Bool
		done    = make(chan struct{}, 1)
	)
	s := &heightSetter{apply: func(h int) {
		if !busy.CompareAndSwap(false, true) {
			t.Error("two heights applied at once")
		}
		time.Sleep(time.Millisecond) // as a trip to the main thread
		mu.Lock()
		applied = append(applied, h)
		mu.Unlock()
		busy.Store(false)
		if h == 300 {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	}}
	for h := 101; h <= 300; h++ {
		s.set(h)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the last height was never applied")
	}
	// Let a late call, if any, land.
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	defer mu.Unlock()
	if got := applied[len(applied)-1]; got != 300 {
		t.Errorf("the window ended at %d, want 300 (applied %v)", got, applied)
	}
	for i := 1; i < len(applied); i++ {
		if applied[i] <= applied[i-1] {
			t.Errorf("heights applied out of order: %v", applied)
			break
		}
	}
}
