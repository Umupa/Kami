// Package selection reads the text selected in the frontmost app: through
// Accessibility first, and by a synthetic Cmd-C when that gives nothing,
// with the clipboard put back exactly as it was.
package selection

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/Umupa/Gloss/internal/macos"
)

// System is what reading a selection takes from macOS.
type System interface {
	FrontmostIsSelf() bool
	SecureInput() bool
	Focused(timeout time.Duration) (macos.Focused, error)
	// PressCopy posts Cmd-C to the frontmost app.
	PressCopy()
	Clipboard
}

// Clipboard is the system clipboard.
type Clipboard interface {
	ChangeCount() int
	String() string
	Snapshot() macos.Snapshot
	Restore(s macos.Snapshot, transient bool) int
}

// Reader reads selections. The zero durations take the defaults.
type Reader struct {
	Sys System
	// AXTimeout bounds every Accessibility call.
	AXTimeout time.Duration
	// CopyWait is how long the copy fallback waits for the clipboard.
	CopyWait time.Duration
	// LateWindow is how long a copy that comes after CopyWait is still put
	// back.
	LateWindow time.Duration
	// Poll is how often the clipboard is looked at meanwhile.
	Poll time.Duration
}

const (
	defaultAXTimeout  = 150 * time.Millisecond
	defaultCopyWait   = 50 * time.Millisecond
	defaultLateWindow = time.Second
	defaultPoll       = 5 * time.Millisecond
)

func orDefault(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}
	return def
}

// Read returns the selected text, trimmed, or "" when there is none or it
// cannot be read: the panel itself in front, a password field, secure
// input, or no Accessibility access.
func (r *Reader) Read(ctx context.Context) string {
	sys := r.Sys
	if sys.FrontmostIsSelf() {
		return ""
	}
	f, err := sys.Focused(orDefault(r.AXTimeout, defaultAXTimeout))
	switch {
	case errors.Is(err, macos.ErrNoAccessibility):
		// Synthetic key events need the same access.
		return ""
	case err == nil && f.Secure():
		return ""
	case err == nil:
		if s := strings.TrimSpace(f.Selected); s != "" {
			return s
		}
	}
	// The selection may be outside the focused element, as in Telegram,
	// where the focus stays in the message field; or the app may not answer
	// Accessibility at all.
	if sys.SecureInput() {
		return ""
	}
	text := r.copy(ctx)
	if err == nil && copiedLine(f, text) {
		return ""
	}
	return strings.TrimSpace(text)
}

// copy sends Cmd-C and returns what it copied, putting the clipboard back.
func (r *Reader) copy(ctx context.Context) string {
	sys := r.Sys
	before := sys.ChangeCount()
	snap := sys.Snapshot()
	sys.PressCopy()

	poll := orDefault(r.Poll, defaultPoll)
	deadline := time.Now().Add(orDefault(r.CopyWait, defaultCopyWait))
	for sys.ChangeCount() == before {
		if time.Now().After(deadline) || ctx.Err() != nil {
			// The copy may still come; put the clipboard back if it does.
			go r.guard(snap, before)
			return ""
		}
		time.Sleep(poll)
	}
	text := sys.String()
	restored := sys.Restore(snap, true)
	go r.guard(snap, restored)
	return text
}

// guard puts the clipboard back once more if a copy lands after count, within
// the late window, so that a slow app's copy does not stay on it.
func (r *Reader) guard(snap macos.Snapshot, count int) {
	sys := r.Sys
	poll := orDefault(r.Poll, defaultPoll)
	deadline := time.Now().Add(orDefault(r.LateWindow, defaultLateWindow))
	for time.Now().Before(deadline) {
		time.Sleep(poll)
		if sys.ChangeCount() != count {
			sys.Restore(snap, true)
			return
		}
	}
}

// copiedLine reports a copy that is the whole line an editor copies when
// nothing is selected: the focused element says its selection is empty, and
// the copied text is one of its lines.
func copiedLine(f macos.Focused, text string) bool {
	if !f.HasSelection || f.SelectedLength != 0 || text == "" {
		return false
	}
	line := strings.TrimRight(text, "\r\n")
	if line == "" || strings.Contains(line, "\n") {
		return false
	}
	for l := range strings.Lines(f.Value) {
		if strings.TrimRight(l, "\r\n") == line {
			return true
		}
	}
	return false
}
