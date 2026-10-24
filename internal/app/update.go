package app

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"github.com/Umupa/Kami/internal/macos"
	"github.com/Umupa/Kami/internal/panel"
)

// Kami looks for a newer version a little after it starts and then every
// few hours (requirements F14). MyGo's Updater checks the signed manifest of
// the latest GitHub release, downloads and verifies the archive, and swaps
// the bundle; the new version runs once Kami restarts. A bundle swapped
// under the running app fails macOS's permission checks until then, so the
// download starts only when the user asks for the restart.
const (
	updateFirstCheck = 5 * time.Second
	updateEvery      = 6 * time.Hour
	updateCheckLimit = 30 * time.Second
	updateLimit      = 10 * time.Minute
)

// skippedUpdate is the user default holding the version the user skipped.
const skippedUpdate = "skippedUpdate"

// updates holds what Kami knows of a newer version. The checks and the
// download write it on their goroutines; the panel and the menu read it on
// the main thread.
type updates struct {
	log *slog.Logger
	// changed runs on the main thread after the state changes.
	changed func()

	mu      sync.Mutex
	found   *mygo.Update
	state   panel.UpdateState
	later   bool
	skipped string
	// asked is the phase of a check the user asked for while no newer
	// version is known; 0 once they close its screen.
	asked panel.UpdatePhase
}

func newUpdates(log *slog.Logger, changed func()) *updates {
	return &updates{log: log, changed: changed, skipped: macos.DefaultString(skippedUpdate)}
}

// run checks for a newer version until ctx ends. Builds without updates,
// such as `mygo dev`, never check.
func (u *updates) run(ctx context.Context) {
	if !mygo.Updater.Enabled() {
		u.log.Info("updates disabled for this build")
		return
	}
	wait := updateFirstCheck
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = updateEvery
		u.check(ctx)
	}
}

// check looks for a newer version unless one is downloading or installed,
// and reports whether it could tell.
func (u *updates) check(ctx context.Context) bool {
	if u.installing() {
		return true
	}
	ctx, cancel := context.WithTimeout(ctx, updateCheckLimit)
	defer cancel()
	found, err := mygo.Updater.Check(ctx)
	if err != nil {
		u.log.Warn("update check", "err", err)
		return false
	}
	if found == nil {
		u.log.Info("update check", "latest", true)
		return true
	}
	u.log.Info("update found", "version", found.Version)
	u.mu.Lock()
	if u.state.Version != found.Version {
		u.later = false
	}
	u.found = found
	u.state = panel.UpdateState{Phase: panel.UpdateAvailable, Current: mygo.App.Version(), Version: found.Version, Notes: found.Notes}
	u.mu.Unlock()
	mygo.RunOnMain(u.changed)
	return true
}

func (u *updates) installing() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state.Phase == panel.UpdateDownloading || u.state.Phase == panel.UpdateReady
}

// Check looks for a newer version now, as the user asked from the menu or
// with /update; its screen shows the outcome. A version being downloaded or
// installed needs no check.
func (u *updates) Check() {
	if u.installing() {
		return
	}
	if !mygo.Updater.Enabled() {
		u.setAsked(panel.UpdateUnsupported)
		return
	}
	u.setAsked(panel.UpdateChecking)
	go func() {
		phase := panel.UpdateCheckFailed
		if u.check(context.Background()) {
			phase = panel.UpdateLatest
		}
		u.mu.Lock()
		switch {
		case u.state.Phase != 0:
			// The newer version takes the screen over.
			u.asked = 0
		case u.asked == panel.UpdateChecking:
			// Closed while checking, nothing waits for the outcome.
			u.asked = phase
		}
		u.mu.Unlock()
		mygo.RunOnMain(u.changed)
	}()
}

func (u *updates) setAsked(phase panel.UpdatePhase) {
	u.mu.Lock()
	u.asked = phase
	u.mu.Unlock()
	mygo.RunOnMain(u.changed)
}

// State returns the newer version, or else the check the user asked for;
// false when there is neither.
func (u *updates) State() (panel.UpdateState, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return shown(u.state, u.asked, mygo.App.Version())
}

func shown(s panel.UpdateState, asked panel.UpdatePhase, current string) (panel.UpdateState, bool) {
	switch {
	case s.Phase != 0:
		return s, true
	case asked != 0:
		return panel.UpdateState{Phase: asked, Current: current}, true
	}
	return s, false
}

// Offer returns the newer version to show as the panel opens.
func (u *updates) Offer() (panel.UpdateState, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state, offered(u.state, u.later, u.skipped)
}

// offered reports whether the panel offers s as it opens: not once the user
// put it off for this run, nor a version they skipped before downloading it.
func offered(s panel.UpdateState, later bool, skipped string) bool {
	switch {
	case s.Phase == 0, later:
		return false
	case s.Phase == panel.UpdateAvailable, s.Phase == panel.UpdateFailed:
		return s.Version != skipped
	}
	return true
}

// Install downloads the newer version and puts it in place.
func (u *updates) Install() {
	u.mu.Lock()
	found := u.found
	if found == nil || u.state.Phase == panel.UpdateDownloading || u.state.Phase == panel.UpdateReady {
		u.mu.Unlock()
		return
	}
	u.state.Phase, u.state.Progress = panel.UpdateDownloading, -1
	u.mu.Unlock()
	u.changed()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updateLimit)
		defer cancel()
		err := found.Install(ctx, func(done, total int64) {
			if total <= 0 {
				return
			}
			u.mu.Lock()
			u.state.Progress = float64(done) / float64(total)
			u.mu.Unlock()
			mygo.RunOnMain(u.changed)
		})
		u.mu.Lock()
		if err != nil {
			u.log.Error("update install", "version", found.Version, "err", err)
			u.state.Phase = panel.UpdateFailed
		} else {
			u.log.Info("update installed", "version", found.Version)
			u.state.Phase = panel.UpdateReady
		}
		u.mu.Unlock()
		mygo.RunOnMain(u.changed)
	}()
}

// Later puts the newer version off for this run, and closes a check.
func (u *updates) Later() {
	u.mu.Lock()
	u.later = true
	u.asked = 0
	u.mu.Unlock()
}

// Skip puts the newer version off until a later one comes out.
func (u *updates) Skip() {
	u.mu.Lock()
	u.skipped = u.state.Version
	u.mu.Unlock()
	macos.SetDefaultString(skippedUpdate, u.skipped)
	u.changed()
}

// menuLabel is the menu bar's item for the newer version; empty hides it.
func (u *updates) menuLabel() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return updateLabel(u.state, u.skipped)
}

func updateLabel(s panel.UpdateState, skipped string) string {
	switch s.Phase {
	case panel.UpdateAvailable:
		if s.Version == skipped {
			return ""
		}
		return "更新到 " + s.Version + "…"
	case panel.UpdateDownloading:
		if s.Progress < 0 {
			return "正在下载更新…"
		}
		return fmt.Sprintf("正在下载更新 %d%%", int(s.Progress*100))
	case panel.UpdateReady:
		return "重启以完成更新"
	case panel.UpdateFailed:
		if s.Version == skipped {
			return ""
		}
		return "更新失败，重试…"
	}
	return ""
}
