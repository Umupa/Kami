package panel

import "sync"

// heightSetter sets the window's height from off the main thread, one call
// at a time and to the height asked for last. Setting it takes trips to the
// main thread, and calls racing each other there would land in any order:
// as the panel grows with a reply, an earlier height could land last and
// stay.
type heightSetter struct {
	apply func(height int)

	mu      sync.Mutex
	want    int
	running bool
}

// set asks for height h, replacing any not yet applied.
func (s *heightSetter) set(h int) {
	s.mu.Lock()
	s.want = h
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	go s.run()
}

// run applies the height asked for until no other is asked for meanwhile.
func (s *heightSetter) run() {
	s.mu.Lock()
	for {
		h := s.want
		s.mu.Unlock()
		s.apply(h)
		s.mu.Lock()
		if s.want == h {
			s.running = false
			s.mu.Unlock()
			return
		}
	}
}
