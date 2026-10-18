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
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/llm"
	"github.com/Umupa/cida-next/internal/mode"
	"github.com/Umupa/cida-next/internal/session"
)

var update = flag.Bool("update", false, "rewrite the snapshots in testdata")

const testAPIKey = "sk-panel-test-key"

const testConfig = `{"version":1,"models":[{"name":"fast","endpoint":"https://example.com/v1","format":"chat-completions","model":"m","apiKey":"` + testAPIKey + `"}]}`

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
		Store:  config.NewStore(path, mode.Check),
		Client: func(config.Model) llm.Client { return client },
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
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
	h.snapshotNow(name)
}

// snapshotNow saves the frame as it is, mid-animation.
func (h *harness) snapshotNow(name string) {
	h.t.Helper()
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

func TestRequestCarriesModelAPIKey(t *testing.T) {
	var got llm.Request
	h := newHarness(t, clientFunc(func(ctx context.Context, req llm.Request, onText func(string)) error {
		got = req
		onText("ok")
		return nil
	}))
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("source")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()

	if got.Key != testAPIKey {
		t.Errorf("request key = %q, want the model's apiKey", got.Key)
	}
	if got.Model.Name != "fast" {
		t.Errorf("request model = %q, want fast", got.Model.Name)
	}
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

func TestShortWindowGrowsToPanel(t *testing.T) {
	// A window shorter than the panel, as before the first measurement,
	// must still let the panel take its own height for the window to follow.
	h := newHarness(t, script{})
	h.tt.SetSize(800, 1)
	h.tt.Frame()
	h.tt.Frame()
	if h.p.height < 80 {
		t.Errorf("panel measured %d high in a 1-high window; the window would never grow", h.p.height)
	}
}

func TestStaleAfterEditingSource(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"ok"}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("first")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	if h.tt.HasText("原文或模式已改，⏎ 重新执行") {
		t.Error("fresh result marked stale")
	}
	h.tt.Click("原文")
	h.tt.Type(" more")
	h.tt.Frame()
	if !h.tt.HasText("原文或模式已改，⏎ 重新执行") {
		t.Error("result not marked stale after the source changed")
	}
	if h.p.sess.Busy() {
		t.Error("an edit started a request on its own")
	}
}

func TestHeightEases(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"line\n\nline\n\nline\n\nline\n\nline"}})
	h.p.height = initialHeight // as Show sets it before the first measurement
	h.tt.Frame()
	for _, r := range h.sizes {
		if r.Height < 60 {
			t.Fatalf("the window dropped to %d while settling: %v", r.Height, h.sizes)
		}
	}
	h.tt.Click("原文")
	h.tt.Type("x")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.sizes = nil
	h.tt.Advance(40 * time.Millisecond)
	mid := h.p.height
	h.tt.Advance(time.Second)
	end := h.p.height
	if !(mid < end) {
		t.Errorf("height went %d then %d; want it to ease up", mid, end)
	}
	if len(h.sizes) < 2 {
		t.Errorf("resized %d times while easing, want several", len(h.sizes))
	}
}

func TestCodeBlockSpeaksChinese(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"```go\nx := 1\n```\n"}})
	h.tt.SetLocale("zh-Hans")
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("x")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	if h.tt.HasText("Copy") {
		t.Errorf("the code block's button is in English: %q", h.tt.Texts())
	}
}

// limited refuses requests whose messages are longer than limit bytes.
type limited struct {
	limit int
	reqs  *[][]llm.Message
}

func (l limited) Stream(ctx context.Context, req llm.Request, onText func(string)) error {
	n := 0
	for _, m := range req.Messages {
		n += len(m.Content)
	}
	*l.reqs = append(*l.reqs, req.Messages)
	if n > l.limit {
		return &llm.Error{Kind: llm.KindTooLong, Status: 400}
	}
	onText("[" + strings.TrimSpace(req.Messages[len(req.Messages)-1].Content) + "]")
	return nil
}

func TestTooLongSourceIsAnsweredInParts(t *testing.T) {
	var reqs [][]llm.Message
	h := newHarness(t, limited{limit: 25, reqs: &reqs})
	h.tt.Frame()
	h.tt.Click("原文")
	h.p.source = "First paragraph here.\n\nSecond paragraph here.\n\nThird one."
	h.tt.Frame()
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	turn := h.p.sess.Last()
	if turn.Status != session.Done || turn.Parts < 2 {
		t.Fatalf("status %v, parts %d, err %v", turn.Status, turn.Parts, turn.Err)
	}
	if !strings.Contains(turn.Reply, "[First paragraph here.]") || !strings.Contains(turn.Reply, "[Third one.]") {
		t.Errorf("reply = %q", turn.Reply)
	}
}

func TestTooLongConversationKeepsSourceTurn(t *testing.T) {
	var reqs [][]llm.Message
	h := newHarness(t, limited{limit: 1 << 20, reqs: &reqs})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("source")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	for _, ask := range []string{"one", "two", "three"} {
		h.tt.Click("追问")
		h.tt.Type(ask)
		h.tt.Key(0, ui.KeyEnter)
		h.drain()
	}
	// The service now takes less than the conversation.
	h.p.deps.Client = func(config.Model) llm.Client { return limited{limit: 30, reqs: &reqs} }
	h.tt.Click("追问")
	h.tt.Type("four")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	turn := h.p.sess.Last()
	last := reqs[len(reqs)-1]
	if turn.Status != session.Done || turn.Dropped != 3 || len(last) != 3 || last[0].Content != "source" || last[2].Content != "four" {
		t.Errorf("status %v dropped %d, last request %+v", turn.Status, turn.Dropped, last)
	}
}

func TestLongReplyKeepsSourceAndBar(t *testing.T) {
	long := strings.Repeat("一段很长的回复内容，用来把面板撑到最大高度。\n\n", 80)
	h := newHarness(t, script{pieces: []string{long}})
	h.p.place = placeOn(mygo.Rectangle{Width: 1440, Height: 600}) // max 420 high
	h.tt.SetSize(820, 700)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("/operator 查看当前的配置")
	h.p.source = "first line"
	h.tt.Frame()
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Frame()
	bar, _ := h.tt.Find("执行 ⏎")
	if bar.Y < 50 {
		t.Errorf("the mode bar is at %v: the source above it was squeezed by a long reply", bar)
	}
	h.snapshot("long-reply")
}
