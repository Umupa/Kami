package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sync"
)

// Snapshot is what the app runs on.
type Snapshot struct {
	// Config is never nil. A model error keeps the last valid model while
	// valid settings still load; other errors keep the last valid configuration.
	Config *Config
	// Err is what is wrong with the file on disk now; nil when the file is
	// Config.
	Err error
}

// Store holds the configuration read from one file and tells subscribers when
// it changes. Its methods are safe from any goroutine.
type Store struct {
	path  string
	check func(*Config) error

	// reload serializes Reload, so subscribers see snapshots in order.
	reload sync.Mutex

	mu   sync.Mutex
	snap Snapshot
	raw  []byte
	read bool
	subs map[int]func(Snapshot)
	next int
}

// NewStore reads path once. check, when set, is a further validation that
// must pass before a configuration replaces the current one, for rules that
// belong to other packages, such as the completeness of custom modes.
func NewStore(path string, check func(*Config) error) *Store {
	initial := starter
	initial.applyDefaults()
	s := &Store{path: path, check: check, snap: Snapshot{Config: &initial}, subs: map[int]func(Snapshot){}}
	s.Reload()
	return s
}

// Path returns the file the store reads.
func (s *Store) Path() string { return s.path }

// Snapshot returns the current snapshot.
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.snap
}

// Subscribe calls fn, on the goroutine that called Reload, after every change
// of the snapshot. The returned function stops it.
func (s *Store) Subscribe(fn func(Snapshot)) (cancel func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := s.next
	s.next++
	s.subs[id] = fn
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		delete(s.subs, id)
	}
}

// Reload reads the file again. A valid file replaces the configuration; an
// invalid model keeps the last valid model but still loads valid settings.
// Other invalid or missing files keep the configuration and set Err.
// Subscribers are told only when the snapshot changed.
func (s *Store) Reload() Snapshot {
	s.reload.Lock()
	defer s.reload.Unlock()

	data, err := os.ReadFile(s.path)
	s.mu.Lock()
	if err == nil && s.read && bytes.Equal(data, s.raw) {
		snap := s.snap
		s.mu.Unlock()
		return snap
	}
	s.mu.Unlock()

	next := s.Snapshot()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		next.Err = &Error{Problems: []Problem{{Message: "找不到配置文件 " + s.path}}}
	case err != nil:
		next.Err = &Error{Problems: []Problem{{Message: fmt.Sprintf("读不了配置文件：%v", err)}}}
	default:
		c, derr := Decode(data)
		if derr != nil {
			if settings, err := decodeSettings(data); err == nil {
				settings.Model = next.Config.Model
				c = settings
			}
		}
		if c != nil && s.check != nil {
			if err := s.check(c); err != nil {
				c = nil
				derr = errors.Join(derr, err)
			}
		}
		if c != nil {
			next.Config = c
		}
		next.Err = derr
	}

	s.mu.Lock()
	changed := !s.read || next.Config != s.snap.Config || errorText(next.Err) != errorText(s.snap.Err)
	s.snap, s.raw, s.read = next, data, true
	subs := make([]func(Snapshot), 0, len(s.subs))
	for _, fn := range s.subs {
		subs = append(subs, fn)
	}
	s.mu.Unlock()

	if changed {
		for _, fn := range subs {
			fn(next)
		}
	}
	return next
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
