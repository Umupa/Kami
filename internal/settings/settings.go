// Package settings is the settings window. Every setting lives in
// config.json, which agents edit; the window shows the state, runs checks,
// grants permissions, and writes the few settings a person changes by hand.
package settings

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/llm"
)

// Deps are what the window works with.
type Deps struct {
	Store  *config.Store
	Client func(config.Model) llm.Client
	Log    *slog.Logger
	// CopyPrompt copies the configuration prompt for an AI assistant.
	CopyPrompt func()
	// RevealConfig shows config.json in Finder.
	RevealConfig func()
	// PauseHotkeys unregisters the global shortcuts while one is recorded,
	// so that pressing it reaches the window.
	PauseHotkeys func(paused bool)
	// Permissions are the system permissions the app uses.
	Permissions []Permission
}

// Permission is a system permission and how to ask for it.
type Permission struct {
	Name, Purpose string
	Granted       func() bool
	Request       func()
}

// Settings is the settings window. Its methods run on the main thread.
type Settings struct {
	deps Deps
	win  *mygo.Window
	post func(func())

	tab    int
	snap   config.Snapshot
	checks map[string]check
	// recording is the shortcut field waiting for a key, "" when none.
	recording string
	editErr   string
	copied    time.Time
	version   string
}

// check is a model's latest check.
type check struct {
	running bool
	ok      bool
	took    time.Duration
	message string
	at      time.Time
}

// New returns the settings, without a window until Show.
func New(d Deps) *Settings {
	s := &Settings{deps: d, checks: map[string]check{}, version: mygo.App.Version()}
	d.Store.Subscribe(func(snap config.Snapshot) {
		if s.post != nil {
			s.post(func() { s.setSnapshot(snap) })
		}
	})
	return s
}

// Show opens the window, or brings it forward.
func (s *Settings) Show() {
	if s.win == nil {
		s.win = mygo.NewWindow(mygo.WindowOptions{
			Title:     "cida-next 设置",
			Width:     680,
			Height:    520,
			MinWidth:  560,
			MinHeight: 420,
			StateKey:  "settings",
			Content:   ui.View(s.view),
		})
		s.post = s.win.Update
		s.win.OnClosed(func() {
			s.stopRecording()
			s.win, s.post = nil, nil
		})
	}
	s.setSnapshot(s.deps.Store.Snapshot())
	s.win.Show()
	s.win.Focus()
	mygo.App.Focus()
}

func (s *Settings) setSnapshot(snap config.Snapshot) { s.snap = snap }

// runCheck pings a model off the main thread.
func (s *Settings) runCheck(m config.Model) {
	s.checks[m.Name] = check{running: true}
	go func() {
		var c check
		if m.APIKey == "" && m.Auth != config.AuthNone {
			c.message = "还差 API Key。"
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			reply, took, perr := llm.Ping(ctx, s.deps.Client(m), m, m.APIKey)
			c.took = took
			if perr == nil {
				c.ok, c.message = true, "回复「"+reply+"」"
			} else {
				var le *llm.Error
				c.message = "请求没有成功。"
				if errors.As(perr, &le) {
					c.message = "请求没有成功（" + le.Kind.String() + "）。"
				}
			}
		}
		c.at = time.Now()
		if s.post != nil {
			s.post(func() { s.checks[m.Name] = c })
		}
	}()
}

// edit writes a change to config.json; the store picks it up as any edit.
func (s *Settings) edit(fn func(*config.Config)) {
	s.editErr = ""
	if err := config.Edit(s.deps.Store.Path(), fn); err != nil {
		s.editErr = err.Error()
		s.deps.Log.Warn("settings edit", "err", err)
	}
}

func (s *Settings) startRecording(field string) {
	s.recording = field
	if s.deps.PauseHotkeys != nil {
		s.deps.PauseHotkeys(true)
	}
}

func (s *Settings) stopRecording() {
	if s.recording == "" {
		return
	}
	s.recording = ""
	if s.deps.PauseHotkeys != nil {
		s.deps.PauseHotkeys(false)
	}
}
