package selection

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Umupa/Kami/internal/macos"
)

const fakePlainTextType = "public.utf8-plain-text"

type copyMode int

const (
	copyNever copyMode = iota
	copyImmediately
	copyAfterDelay
	copyClearThenWrite
)

type restoreCall struct {
	snapshot  macos.Snapshot
	transient bool
}

// fakeSystem is a System whose app and clipboard are driven by the test.
// The guard introduced by Reader runs on its own goroutine, so every method
// locks.
type fakeSystem struct {
	mu sync.Mutex

	frontmost  bool
	secure     bool
	focused    macos.Focused
	focusedErr error

	contents macos.Snapshot
	changes  int

	copyMode  copyMode
	copyText  string
	copyDelay time.Duration

	presses      int
	restoreCalls []restoreCall
}

var _ System = (*fakeSystem)(nil)

func (f *fakeSystem) FrontmostIsSelf() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.frontmost
}

func (f *fakeSystem) SecureInput() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.secure
}

func (f *fakeSystem) Focused(time.Duration) (macos.Focused, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.focused, f.focusedErr
}

func (f *fakeSystem) PressCopy() {
	f.mu.Lock()
	f.presses++
	mode, text, delay := f.copyMode, f.copyText, f.copyDelay
	f.mu.Unlock()

	switch mode {
	case copyImmediately:
		f.paste(text)
	case copyClearThenWrite:
		f.mu.Lock()
		f.contents = nil
		f.changes++
		f.mu.Unlock()
		fallthrough
	case copyAfterDelay:
		go func() {
			time.Sleep(delay)
			f.paste(text)
		}()
	}
}

func (f *fakeSystem) ChangeCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.changes
}

func (f *fakeSystem) String() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, item := range f.contents {
		for i, typ := range item.Types {
			if typ == fakePlainTextType && i < len(item.Data) {
				return string(item.Data[i])
			}
		}
	}
	return ""
}

func (f *fakeSystem) Snapshot() macos.Snapshot {
	f.mu.Lock()
	defer f.mu.Unlock()
	return cloneSnapshot(f.contents)
}

func (f *fakeSystem) Restore(s macos.Snapshot, transient bool) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.restoreCalls = append(f.restoreCalls, restoreCall{snapshot: cloneSnapshot(s), transient: transient})
	f.contents = cloneSnapshot(s)
	f.changes++
	return f.changes
}

// paste simulates an app writing copied text to the clipboard.
func (f *fakeSystem) paste(text string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.contents = textSnapshot(text)
	f.changes++
}

func (f *fakeSystem) pressCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.presses
}

func (f *fakeSystem) restoreLog() []restoreCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]restoreCall(nil), f.restoreCalls...)
}

func textSnapshot(text string) macos.Snapshot {
	return macos.Snapshot{{
		Types: []string{fakePlainTextType},
		Data:  [][]byte{[]byte(text)},
	}}
}

func sampleSnapshot() macos.Snapshot {
	return macos.Snapshot{
		{
			Types: []string{fakePlainTextType, "public.html"},
			Data:  [][]byte{[]byte("original"), []byte("<b>original</b>")},
		},
		{
			Types: []string{"com.example.custom"},
			Data:  [][]byte{{0, 1, 2, 255}},
		},
	}
}

func cloneSnapshot(s macos.Snapshot) macos.Snapshot {
	if s == nil {
		return nil
	}
	clone := make(macos.Snapshot, len(s))
	for i, item := range s {
		clone[i].Types = append([]string(nil), item.Types...)
		clone[i].Data = make([][]byte, len(item.Data))
		for j, data := range item.Data {
			clone[i].Data[j] = append([]byte(nil), data...)
		}
	}
	return clone
}

func newTestReader(sys System) *Reader {
	return &Reader{
		Sys:        sys,
		AXTimeout:  10 * time.Millisecond,
		CopyWait:   20 * time.Millisecond,
		LateWindow: 100 * time.Millisecond,
		Poll:       time.Millisecond,
	}
}

func waitForRestores(t *testing.T, f *fakeSystem, want int) []restoreCall {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		calls := f.restoreLog()
		if len(calls) >= want {
			return calls
		}
		if time.Now().After(deadline) {
			t.Fatalf("got %d Restore calls, want %d", len(calls), want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestReadFrontmostIsSelf(t *testing.T) {
	f := &fakeSystem{
		frontmost: true,
		focused:   macos.Focused{Selected: "not read"},
		copyMode:  copyImmediately,
		copyText:  "not copied",
	}
	r := newTestReader(f)

	if got := r.Read(context.Background()); got != "" {
		t.Errorf("Read() = %q, want empty", got)
	}
	if got := f.pressCount(); got != 0 {
		t.Errorf("PressCopy called %d times, want 0", got)
	}
}

func TestReadNoAccessibility(t *testing.T) {
	f := &fakeSystem{
		focusedErr: macos.ErrNoAccessibility,
		copyMode:   copyImmediately,
		copyText:   "not copied",
	}
	r := newTestReader(f)

	if got := r.Read(context.Background()); got != "" {
		t.Errorf("Read() = %q, want empty", got)
	}
	if got := f.pressCount(); got != 0 {
		t.Errorf("PressCopy called %d times, want 0", got)
	}
}

func TestReadPasswordField(t *testing.T) {
	tests := []struct {
		name    string
		focused macos.Focused
	}{
		{"role", macos.Focused{Role: "AXSecureTextField", Selected: "secret", HasSelection: true, SelectedLength: 6}},
		{"subrole", macos.Focused{Subrole: "AXSecureTextField", Selected: "secret", HasSelection: true, SelectedLength: 6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeSystem{
				focused:  tt.focused,
				copyMode: copyImmediately,
				copyText: "not copied",
			}
			r := newTestReader(f)

			if got := r.Read(context.Background()); got != "" {
				t.Errorf("Read() = %q, want empty", got)
			}
			if got := f.pressCount(); got != 0 {
				t.Errorf("PressCopy called %d times, want 0", got)
			}
		})
	}
}

func TestReadSelectedText(t *testing.T) {
	f := &fakeSystem{
		focused:  macos.Focused{Selected: "  hello  ", HasSelection: true, SelectedLength: 7},
		copyMode: copyImmediately,
		copyText: "not copied",
	}
	r := newTestReader(f)

	if got := r.Read(context.Background()); got != "hello" {
		t.Errorf("Read() = %q, want %q", got, "hello")
	}
	if got := f.pressCount(); got != 0 {
		t.Errorf("PressCopy called %d times, want 0", got)
	}
}

func TestReadSecureInput(t *testing.T) {
	f := &fakeSystem{
		focused:  macos.Focused{HasSelection: true, SelectedLength: 0},
		secure:   true,
		copyMode: copyImmediately,
		copyText: "not copied",
	}
	r := newTestReader(f)

	if got := r.Read(context.Background()); got != "" {
		t.Errorf("Read() = %q, want empty", got)
	}
	if got := f.pressCount(); got != 0 {
		t.Errorf("PressCopy called %d times, want 0", got)
	}
}

func TestReadCopyFallback(t *testing.T) {
	original := sampleSnapshot()
	f := &fakeSystem{
		contents: cloneSnapshot(original),
		focused:  macos.Focused{HasSelection: true, SelectedLength: 0},
		copyMode: copyImmediately,
		copyText: "copied text",
	}
	r := newTestReader(f)

	if got := r.Read(context.Background()); got != "copied text" {
		t.Errorf("Read() = %q, want %q", got, "copied text")
	}
	if got := f.pressCount(); got != 1 {
		t.Errorf("PressCopy called %d times, want 1", got)
	}
	calls := f.restoreLog()
	if len(calls) != 1 {
		t.Fatalf("got %d Restore calls, want 1", len(calls))
	}
	if !reflect.DeepEqual(calls[0].snapshot, original) {
		t.Errorf("restored snapshot = %#v, want %#v", calls[0].snapshot, original)
	}
	if !calls[0].transient {
		t.Error("restored contents were not marked transient")
	}
	if got := f.String(); got != "original" {
		t.Errorf("String() = %q after Read, want %q", got, "original")
	}
}

func TestReadFocusedElementErrorFallsBack(t *testing.T) {
	original := sampleSnapshot()
	f := &fakeSystem{
		contents:   cloneSnapshot(original),
		focusedErr: errors.New("macos: no focused element"),
		copyMode:   copyImmediately,
		copyText:   "fallback",
	}
	r := newTestReader(f)

	if got := r.Read(context.Background()); got != "fallback" {
		t.Errorf("Read() = %q, want %q", got, "fallback")
	}
	if got := f.pressCount(); got != 1 {
		t.Errorf("PressCopy called %d times, want 1", got)
	}
	calls := f.restoreLog()
	if len(calls) != 1 {
		t.Fatalf("got %d Restore calls, want 1", len(calls))
	}
	if !reflect.DeepEqual(calls[0].snapshot, original) {
		t.Errorf("restored snapshot = %#v, want %#v", calls[0].snapshot, original)
	}
}

func TestReadCopyNever(t *testing.T) {
	f := &fakeSystem{
		contents: sampleSnapshot(),
		focused:  macos.Focused{HasSelection: true, SelectedLength: 0},
		copyMode: copyNever,
	}
	r := newTestReader(f)

	start := time.Now()
	if got := r.Read(context.Background()); got != "" {
		t.Errorf("Read() = %q, want empty", got)
	}
	if elapsed := time.Since(start); elapsed < r.CopyWait/2 || elapsed > r.CopyWait+100*time.Millisecond {
		t.Errorf("Read() returned after %v, want about CopyWait (%v)", elapsed, r.CopyWait)
	}
	if got := f.pressCount(); got != 1 {
		t.Errorf("PressCopy called %d times, want 1", got)
	}
	if calls := f.restoreLog(); len(calls) != 0 {
		t.Errorf("got %d Restore calls without a copy, want 0", len(calls))
	}
}

func TestReadLateCopyAfterWait(t *testing.T) {
	original := sampleSnapshot()
	f := &fakeSystem{
		contents:  cloneSnapshot(original),
		focused:   macos.Focused{HasSelection: true, SelectedLength: 0},
		copyMode:  copyAfterDelay,
		copyDelay: 50 * time.Millisecond,
		copyText:  "late",
	}
	r := newTestReader(f)

	if got := r.Read(context.Background()); got != "" {
		t.Errorf("Read() = %q, want empty", got)
	}
	if got := f.pressCount(); got != 1 {
		t.Errorf("PressCopy called %d times, want 1", got)
	}
	calls := waitForRestores(t, f, 1)
	if !reflect.DeepEqual(calls[0].snapshot, original) {
		t.Errorf("restored snapshot = %#v, want %#v", calls[0].snapshot, original)
	}
	if !calls[0].transient {
		t.Error("late copy was not restored transiently")
	}
	if got := f.String(); got != "original" {
		t.Errorf("String() = %q after the late copy, want %q", got, "original")
	}
}

// Once the requested copy completed, later clipboard changes belong to the
// user and must not be rolled back by its late-copy guard.
func TestReadSuccessfulCopyLeavesNextWriteAlone(t *testing.T) {
	f := &fakeSystem{contents: sampleSnapshot(), copyMode: copyImmediately, copyText: "selected text"}
	r := newTestReader(f)
	if got := r.Read(context.Background()); got != "selected text" {
		t.Fatalf("Read() = %q", got)
	}
	f.paste("next user copy")
	time.Sleep(r.LateWindow + 20*time.Millisecond)
	if got := f.String(); got != "next user copy" {
		t.Fatalf("clipboard = %q, want next user copy", got)
	}
	if calls := f.restoreLog(); len(calls) != 1 {
		t.Fatalf("got %d restores, want 1", len(calls))
	}
}

func TestReadCopyAfterLateWindow(t *testing.T) {
	original := sampleSnapshot()
	f := &fakeSystem{
		contents: cloneSnapshot(original),
		focused:  macos.Focused{HasSelection: true, SelectedLength: 0},
		copyMode: copyImmediately,
		copyText: "first",
	}
	r := newTestReader(f)

	if got := r.Read(context.Background()); got != "first" {
		t.Fatalf("Read() = %q, want %q", got, "first")
	}
	if calls := f.restoreLog(); len(calls) != 1 {
		t.Fatalf("got %d Restore calls after the copy, want 1", len(calls))
	}

	// Let the guard's window close before the app copies again.
	time.Sleep(r.LateWindow + 50*time.Millisecond)
	f.paste("after")

	// Give a guard that is still alive time to act; there should be none.
	time.Sleep(20 * time.Millisecond)
	if calls := f.restoreLog(); len(calls) != 1 {
		t.Errorf("got %d Restore calls after the late window, want 1", len(calls))
	}
	if got := f.String(); got != "after" {
		t.Errorf("String() = %q after the late window, want %q", got, "after")
	}
}

func TestReadCopiedLine(t *testing.T) {
	tests := []struct {
		name           string
		selectedLength int
		copied         string
		want           string
	}{
		{"whole line is not a selection", 0, "line two\n", ""},
		{"unknown selected length", -1, "line two\n", "line two"},
		{"copied text is not a line", 0, "not a line\n", "not a line"},
		{"multi-line copy", 0, "line one\nline two\n", "line one\nline two"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := sampleSnapshot()
			f := &fakeSystem{
				contents: cloneSnapshot(original),
				focused: macos.Focused{
					HasSelection:   true,
					SelectedLength: tt.selectedLength,
					Value:          "line one\nline two\n",
				},
				copyMode: copyImmediately,
				copyText: tt.copied,
			}
			r := newTestReader(f)

			if got := r.Read(context.Background()); got != tt.want {
				t.Errorf("Read() = %q, want %q", got, tt.want)
			}
			if got := f.pressCount(); got != 1 {
				t.Errorf("PressCopy called %d times, want 1", got)
			}
			calls := f.restoreLog()
			if len(calls) != 1 {
				t.Fatalf("got %d Restore calls, want 1", len(calls))
			}
			if !reflect.DeepEqual(calls[0].snapshot, original) {
				t.Errorf("restored snapshot = %#v, want %#v", calls[0].snapshot, original)
			}
			if !calls[0].transient {
				t.Error("restored contents were not marked transient")
			}
			if got := f.String(); got != "original" {
				t.Errorf("String() = %q after Read, want %q", got, "original")
			}
		})
	}
}

func TestReadCancelledContext(t *testing.T) {
	f := &fakeSystem{
		contents: sampleSnapshot(),
		focused:  macos.Focused{HasSelection: true, SelectedLength: 0},
		copyMode: copyNever,
	}
	r := newTestReader(f)
	r.CopyWait = time.Second

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if got := r.Read(ctx); got != "" {
		t.Errorf("Read() = %q, want empty", got)
	}
	if elapsed := time.Since(start); elapsed >= 250*time.Millisecond {
		t.Errorf("Read() waited %v with a cancelled context, want much less than %v", elapsed, r.CopyWait)
	}
}

func TestReadEmptyClipboard(t *testing.T) {
	f := &fakeSystem{
		focused:  macos.Focused{HasSelection: true, SelectedLength: 0},
		copyMode: copyImmediately,
		copyText: "copied",
	}
	r := newTestReader(f)

	if got := r.Read(context.Background()); got != "copied" {
		t.Errorf("Read() = %q, want %q", got, "copied")
	}
	calls := f.restoreLog()
	if len(calls) != 1 {
		t.Fatalf("got %d Restore calls, want 1", len(calls))
	}
	if len(calls[0].snapshot) != 0 {
		t.Errorf("restored %d items, want an empty clipboard", len(calls[0].snapshot))
	}
	if !calls[0].transient {
		t.Error("empty clipboard was not restored transiently")
	}
	if got := f.String(); got != "" {
		t.Errorf("String() = %q after Read, want empty", got)
	}
}

// Native apps can publish a copy asynchronously, after the original 50 ms
// budget, or clear the pasteboard before publishing its text.
func TestReadAsyncCopy(t *testing.T) {
	for _, tt := range []struct {
		name  string
		mode  copyMode
		delay time.Duration
	}{
		{"slow application", copyAfterDelay, 100 * time.Millisecond},
		{"clear before write", copyClearThenWrite, 20 * time.Millisecond},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeSystem{contents: sampleSnapshot(), copyMode: tt.mode, copyDelay: tt.delay, copyText: "selected text"}
			r := newTestReader(f)
			r.CopyWait = 0 // Exercise the production default.
			if got := r.Read(context.Background()); got != "selected text" {
				t.Fatalf("Read() = %q, want selected text", got)
			}
			if f.pressCount() != 1 {
				t.Fatal("copy must be sent exactly once")
			}
			if got := f.String(); got != "original" {
				t.Fatalf("clipboard = %q, want original", got)
			}
		})
	}
}

func TestReadLateCopyWaitsForTextAfterClear(t *testing.T) {
	f := &fakeSystem{contents: sampleSnapshot(), copyMode: copyNever}
	r := newTestReader(f)
	if got := r.Read(context.Background()); got != "" {
		t.Fatalf("Read() = %q", got)
	}
	f.mu.Lock()
	f.contents = nil
	f.changes++
	f.mu.Unlock()
	time.Sleep(10 * r.Poll)
	f.paste("late selected text")
	waitForRestores(t, f, 1)
	if got := f.String(); got != "original" {
		t.Fatalf("clipboard = %q, want original", got)
	}
}
