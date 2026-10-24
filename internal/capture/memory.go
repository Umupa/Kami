package capture

import (
	"runtime/debug"
	"sync"
	"time"
)

const memoryIdleDelay = 30 * time.Second

type memoryTimer interface{ Stop() bool }

// idleMemory returns temporary Go pages after a burst of captures. The runtime
// operation is process-wide and runs off the UI thread; native caches are separate.
type idleMemory struct {
	mu         sync.Mutex
	timer      memoryTimer
	generation uint64
	closed     bool
	after      func(time.Duration, func()) memoryTimer
	release    func()
}

func newIdleMemory() *idleMemory {
	return &idleMemory{
		after:   func(d time.Duration, fn func()) memoryTimer { return time.AfterFunc(d, fn) },
		release: debug.FreeOSMemory,
	}
}

func (m *idleMemory) stopLocked() {
	m.generation++
	if m.timer != nil {
		m.timer.Stop()
		m.timer = nil
	}
}

func (m *idleMemory) cancel() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	pending := m.timer != nil
	m.stopLocked()
	return pending
}

func (m *idleMemory) schedule() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.stopLocked()
	generation := m.generation
	m.timer = m.after(memoryIdleDelay, func() {
		m.mu.Lock()
		if m.closed || m.generation != generation {
			m.mu.Unlock()
			return
		}
		m.timer = nil
		m.mu.Unlock()
		// A runtime collection already started cannot be canceled by a new capture.
		m.release()
	})
}

func (m *idleMemory) close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	m.stopLocked()
}
