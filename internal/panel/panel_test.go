package panel

import (
	"context"
	"flag"
	"image/png"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/keyring"
	"github.com/Umupa/cida-next/internal/llm"
	"github.com/Umupa/cida-next/internal/mode"
)

var update = flag.Bool("update", false, "rewrite the snapshots in testdata")

const testConfig = `{"version":1,"models":[{"name":"fast","endpoint":"https://example.com/v1","format":"chat-completions","model":"m"}]}`

// script is a client that replies with fixed pieces.
type script struct{ pieces []string }

func (s script) Stream(ctx context.Context, req llm.Request, onText func(string)) error {
	for _, p := range s.pieces {
		onText(p)
	}
	return nil
}

// harness drives a panel without a window: what its goroutines post waits in
// a queue until drain applies it on the test's goroutine.
type harness struct {
	t     *testing.T
	p     *Panel
	tt    *ui.Tester
	mu    sync.Mutex
	queue []func()
	sizes []mygo.Rectangle
	clip  string
}

func newHarness(t *testing.T, client llm.Client) *harness {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, config.FileName)
	if err := os.WriteFile(path, []byte(testConfig), 0o600); err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t}
	h.p = newPanel(Deps{
		Store:   config.NewStore(path, mode.Check),
		Keyring: keyring.NewMemory(),
		Client:  func(config.Model) llm.Client { return client },
		Log:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	h.p.post = func(fn func()) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.queue = append(h.queue, fn)
	}
	h.p.resize = func(r mygo.Rectangle) { h.sizes = append(h.sizes, r) }
	h.p.writeClipboard = func(s string) { h.clip = s }
	h.p.place = placeOn(mygo.Rectangle{Width: 1440, Height: 875})
	h.tt = ui.NewTester(h.p.view, 820, 700)
	return h
}

// drain applies what was posted until the latest turn ends.
func (h *harness) drain() {
	h.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		h.mu.Lock()
		q := h.queue
		h.queue = nil
		h.mu.Unlock()
		for _, fn := range q {
			fn()
		}
		h.tt.Frame()
		if h.p.sess == nil || !h.p.sess.Busy() {
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatal("the turn did not end")
		}
		time.Sleep(time.Millisecond)
	}
}

func (h *harness) snapshot(name string) {
	h.t.Helper()
	h.tt.Advance(2 * time.Second) // let the reveal finish
	path := filepath.Join("testdata", name+".png")
	if !*update {
		return
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		h.t.Fatal(err)
	}
	f, err := os.Create(path)
	if err != nil {
		h.t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, h.tt.Image()); err != nil {
		h.t.Fatal(err)
	}
}

func TestEnterRunsDefaultMode(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"## 标题\n\n", "| a | b |\n|---|---|\n| 1 | 2 |\n"}})
	h.tt.Frame()
	if err := h.tt.Click("原文"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("Hello, world")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()

	if h.p.sess == nil {
		t.Fatal("Enter started no session")
	}
	if got := h.p.sess.Mode.ID; got != "translate" {
		t.Errorf("mode = %s, want translate", got)
	}
	if got := h.p.sess.Last().Reply; !strings.Contains(got, "| 1 | 2 |") {
		t.Errorf("reply = %q", got)
	}
	if h.p.source != "Hello, world" {
		t.Errorf("source = %q; Enter must not insert a newline", h.p.source)
	}
	if len(h.sizes) == 0 {
		t.Error("the window was never fitted to the panel")
	}
	h.snapshot("result")
}

func TestTabCyclesModes(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Key(0, ui.KeyTab)
	if got := h.p.modes[h.p.modeIdx].ID; got != "improve" {
		t.Errorf("after Tab: %s, want improve", got)
	}
	h.tt.Key(ui.Shift, ui.KeyTab)
	h.tt.Key(ui.Shift, ui.KeyTab)
	if got := h.p.modes[h.p.modeIdx].ID; got != "ask" {
		t.Errorf("after two Shift-Tabs: %s, want ask", got)
	}
	h.snapshot("empty")
}

func TestFollowUpSendsConversation(t *testing.T) {
	var got []llm.Message
	client := clientFunc(func(ctx context.Context, req llm.Request, onText func(string)) error {
		got = req.Messages
		onText("ok")
		return nil
	})
	h := newHarness(t, client)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("source text")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	if err := h.tt.Click("追问"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("shorter")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()

	if len(got) != 3 || got[0].Content != "source text" || got[1].Content != "ok" || got[2].Content != "shorter" {
		t.Errorf("messages = %+v", got)
	}
	h.snapshot("follow-up")
}

type clientFunc func(ctx context.Context, req llm.Request, onText func(string)) error

func (f clientFunc) Stream(ctx context.Context, req llm.Request, onText func(string)) error {
	return f(ctx, req, onText)
}

func TestCopy(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"**reply**"}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("source text")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()

	// The source has the focus but nothing selected: Cmd-C copies the reply.
	h.tt.Key(ui.Cmd, ui.KeyC)
	if got := h.clip; got != "**reply**" {
		t.Errorf("Cmd-C without a selection copied %q, want the reply's Markdown", got)
	}
	// With text selected in the source, Cmd-C copies that text.
	h.tt.Key(ui.Cmd, ui.KeyA)
	h.tt.Key(ui.Cmd, ui.KeyC)
	if got := h.tt.Clipboard(); got != "source text" {
		t.Errorf("Cmd-C with a selection copied %q, want the selection", got)
	}
}

func TestBusy(t *testing.T) {
	var busy []bool
	h := newHarness(t, script{pieces: []string{"ok"}})
	h.p.deps.OnBusy = func(b bool) { busy = append(busy, b) }
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("x")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	if len(busy) != 2 || !busy[0] || busy[1] {
		t.Errorf("busy = %v, want [true false]", busy)
	}
}
