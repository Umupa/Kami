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

func (u *updates) check(ctx context.Context) {
	u.mu.Lock()
	busy := u.state.Phase == panel.UpdateDownloading || u.state.Phase == panel.UpdateReady
	u.mu.Unlock()
	if busy {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, updateCheckLimit)
	defer cancel()
	found, err := mygo.Updater.Check(ctx)
	if err != nil {
		u.log.Warn("update check", "err", err)
		return
	}
	if found == nil {
		u.log.Info("update check", "latest", true)
		return
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
}

// State returns the newer version; false when there is none.
func (u *updates) State() (panel.UpdateState, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.state, u.state.Phase != 0
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

// Later puts the newer version off for this run.
func (u *updates) Later() {
	u.mu.Lock()
	u.later = true
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
