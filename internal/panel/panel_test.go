package panel

import (
	"context"
	"flag"
	"image/png"
	"io"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egoist/mygo"
	_ "github.com/egoist/mygo/locales/zh-Hans"
	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/design"
	"github.com/Umupa/Kami/internal/llm"
	"github.com/Umupa/Kami/internal/mode"
	"github.com/Umupa/Kami/internal/session"
)

var update = flag.Bool("update", false, "rewrite the snapshots in testdata")

const testAPIKey = "sk-panel-test-key"

const testConfig = `{"version":1,"model":{"endpoint":"https://example.com/v1","format":"chat-completions","model":"m","apiKey":"` + testAPIKey + `"}}`

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
	return newHarnessWith(t, client, testConfig)
}

// newHarnessWith drives a panel on config.json as given.
func newHarnessWith(t *testing.T, client llm.Client, configJSON string) *harness {
	t.Helper()
	if err := design.LoadFont(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, config.FileName)
	if err := os.WriteFile(path, []byte(configJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	h := &harness{t: t}
	h.p = newPanel(Deps{
		Store:  config.NewStore(path, mode.Check),
		Client: func(config.Model) llm.Client { return client },
		Log:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		// The notes go next to the test's config.json; deleting one
		// removes it, as there is no Trash to test with.
		NotesDir: func(*config.Config) string { return filepath.Join(dir, "notes") },
	})
	h.p.background = func(fn func()) { fn() }
	h.p.after = func(time.Duration, func()) {}
	h.p.post = func(fn func()) {
		h.mu.Lock()
		defer h.mu.Unlock()
		h.queue = append(h.queue, fn)
	}
	h.p.resize = func(r mygo.Rectangle) { h.sizes = append(h.sizes, r) }
	h.p.writeClipboard = func(s string) { h.clip = s }
	h.p.place = placeOn(mygo.Rectangle{Width: 1440, Height: 875})
	h.tt = ui.NewTester(h.p.view, 820, 700)
	h.tt.SetLocale("zh-Hans")
	return h
}

// drain applies what was posted until the latest turn ends.
// settle lays out a frame at the height the panel went to.
func (h *harness) settle() {
	h.tt.Advance(16 * time.Millisecond)
	h.tt.Frame()
	h.tt.Frame()
}

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
		if !h.p.busy() {
			h.settle()
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
	if got.Model.Model != "m" {
		t.Errorf("request model = %q, want m", got.Model.Model)
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
	if !h.p.noteMode() {
		t.Errorf("after two Shift-Tabs: mode %d, want notes, after the modes", h.p.modeIdx)
	}
	h.tt.Key(ui.Shift, ui.KeyTab)
	if got := h.p.modes[h.p.modeIdx].ID; got != "ask" {
		t.Errorf("after three Shift-Tabs: %s, want ask", got)
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
	if h.tt.HasText("原文已改") {
		t.Error("fresh result marked stale")
	}
	h.tt.Click("原文")
	h.tt.Type(" more")
	h.tt.Frame()
	if !h.tt.HasText("原文已改") {
		t.Error("result not marked stale after the source changed")
	}
	if h.p.sess.Busy() {
		t.Error("an edit started a request on its own")
	}
}

func TestEachModeKeepsItsResult(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	client := clientFunc(func(ctx context.Context, req llm.Request, onText func(string)) error {
		if calls.Add(1) == 1 { // improve's request
			<-release
			onText("润色的结果")
			return nil
		}
		onText("其他模式的结果")
		return nil
	})
	h := newHarness(t, client)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("first")
	h.tt.Key(0, ui.KeyTab) // improve
	h.tt.Key(0, ui.KeyEnter)
	h.tt.Frame()

	// The reply goes on streaming while another mode shows.
	h.tt.Key(0, ui.KeyTab)
	h.tt.Frame()
	if h.p.sess != nil {
		t.Fatalf("%s shows improve's result", h.p.modes[h.p.modeIdx].ID)
	}
	h.tt.Key(0, ui.KeyEnter)
	close(release)
	h.drain()
	if !h.tt.HasText("其他模式的结果") || h.tt.HasText("润色的结果") {
		t.Errorf("after running a second mode: %q", h.tt.Texts())
	}

	h.tt.Key(ui.Shift, ui.KeyTab) // back to improve
	h.tt.Frame()
	if !h.tt.HasText("润色的结果") || h.tt.HasText("其他模式的结果") {
		t.Errorf("back in improve: %q", h.tt.Texts())
	}
	if h.tt.HasText("原文已改") {
		t.Error("improve's own result marked stale")
	}

	// A new source marks every mode's result stale.
	h.tt.Click("原文")
	h.tt.Type(" more")
	h.tt.Key(0, ui.KeyTab)
	h.tt.Frame()
	if !h.tt.HasText("原文已改") {
		t.Error("a result for the earlier source not marked stale")
	}
}

func TestWindowFollowsContentInTheSameFrame(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"line"}})
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

	// The content grows; one drawn frame asks the window for its height.
	turn := h.p.sess.Last()
	turn.Reply += strings.Repeat("\n\nmore", 6)
	h.sizes = nil
	h.tt.Frame()
	if len(h.sizes) == 0 {
		t.Fatal("the window was not asked to grow")
	}
	want := int(math.Ceil(float64(h.p.panelH)))
	if got := h.sizes[len(h.sizes)-1].Height; got != want {
		t.Errorf("asked for %d, the content drawn is %d: the window lags", got, want)
	}

	// A shorter reply in the same session leaves it as tall.
	h.settle()
	tall := h.p.height
	turn.Reply = "line"
	h.sizes = nil
	h.tt.Frame()
	if len(h.sizes) != 0 || h.p.height != tall {
		t.Errorf("a shorter reply asked %v, from %d", h.sizes, tall)
	}

	// A new session shrinks it at once.
	h.sizes = nil
	h.tt.Key(ui.Cmd, ui.KeyN)
	h.tt.Frame()
	if len(h.sizes) == 0 || h.sizes[len(h.sizes)-1].Height != h.p.natural || h.p.height >= tall {
		t.Errorf("after Cmd-N: asked %v for content %v, from %d", h.sizes, h.p.natural, tall)
	}
}

func TestHeightGrowsOnlyInSteps(t *testing.T) {
	release := make(chan struct{})
	client := clientFunc(func(ctx context.Context, req llm.Request, onText func(string)) error {
		<-release
		return nil
	})
	h := newHarness(t, client)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("x")
	h.tt.Key(0, ui.KeyEnter)
	h.tt.Frame()

	// Streaming in, a line at a time, the window grows a few lines at a
	// time.
	turn := h.p.sess.Last()
	h.sizes = nil
	const lines = 12
	for range lines {
		turn.Reply += "a line\n\n"
		for range 4 {
			h.tt.Advance(16 * time.Millisecond)
			h.tt.Frame()
		}
	}
	// The window goes to each new height in one step, as each resize
	// shows a stale frame until one of the new size is drawn: it never
	// shrinks, and asks no more than once for each line that came.
	for i := 1; i < len(h.sizes); i++ {
		if h.sizes[i].Height <= h.sizes[i-1].Height {
			t.Fatalf("the window went from %d to %d: %v", h.sizes[i-1].Height, h.sizes[i].Height, h.sizes)
		}
	}
	if len(h.sizes) == 0 || len(h.sizes) > lines {
		t.Errorf("%d resizes for %d lines: %v", len(h.sizes), lines, h.sizes)
	}
	close(release)
	h.drain()

	// Another mode, with no result, keeps the height: the source fills it
	// and the bar stays at the bottom.
	tall := h.p.height
	h.tt.Key(0, ui.KeyTab)
	h.tt.Frame()
	h.tt.Frame()
	if h.p.height != tall {
		t.Errorf("a mode with no result: %d high, was %d", h.p.height, tall)
	}
	bar, _ := h.tt.Find("执行")
	if float32(tall)-(bar.Y+bar.H) > 20 {
		t.Errorf("the bar ends at %v in a panel %d high: it should keep to the bottom", bar.Y+bar.H, tall)
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
	bar, _ := h.tt.Find("执行")
	if bar.Y < 50 {
		t.Errorf("the mode bar is at %v: the source above it was squeezed by a long reply", bar)
	}
	h.snapshot("long-reply")
}

func TestSwitchInputAndCaret(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"ok"}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("hello")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Key(ui.Cmd, ui.KeyJ)
	if !h.tt.Focused("追问") {
		t.Error("⌘J did not move to the follow-up")
	}
	h.tt.Key(ui.Cmd, ui.KeyJ)
	if !h.tt.Focused("原文") {
		t.Error("⌘J did not move back to the source")
	}

	h.p.source = ""
	h.tt.Frame()
	h.tt.Type("/op")
	h.tt.Key(0, ui.KeyTab)
	h.tt.Frame()
	h.tt.Type("主题")
	if h.p.source != "/operator 主题" {
		t.Errorf("typing after picking /operator gave %q; the caret was not at the end", h.p.source)
	}

	// A topic sends its question, and the keyboard goes to the follow-up;
	// ⌘J goes back to the source after its text.
	h.p.source = "/operator "
	h.p.caretToEnd = true
	h.tt.Frame()
	h.tt.Key(0, ui.KeyDown)
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Frame()
	if !h.tt.Focused("追问") {
		t.Fatalf("the topic left the keyboard in the source: %q", h.p.source)
	}
	h.tt.Key(ui.Cmd, ui.KeyJ)
	h.tt.Frame()
	h.tt.Type("！")
	if !strings.HasSuffix(h.p.source, "颜色的值。！") {
		t.Errorf("typing after ⌘J gave %q; the caret was not at the end", h.p.source)
	}
}

func TestTranslateLabel(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	if h.tt.HasText("翻译 → English") {
		t.Error("an empty source shows a direction")
	}
	h.tt.Click("原文")
	h.tt.Type("你好世界")
	h.tt.Frame()
	if !h.tt.HasText("翻译 → English") {
		t.Errorf("Chinese source: %q", h.tt.Texts())
	}
	h.p.source = "hello world"
	h.tt.Frame()
	if !h.tt.HasText("翻译 → 简体中文") {
		t.Errorf("English source: %q", h.tt.Texts())
	}
	h.tt.Key(0, ui.KeyTab)
	if h.tt.HasText("翻译 → 简体中文") || !h.tt.HasText("翻译") {
		t.Error("the direction shows while translate is not the mode")
	}
}

func TestSplitterResizesSource(t *testing.T) {
	h := newHarness(t, script{pieces: []string{strings.Repeat("结果。\n\n", 40)}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.p.source = strings.Repeat("很长的原文一行。\n", 20)
	h.tt.Frame()
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Frame()
	compact := h.p.sourceBox.H
	if compact > 120 {
		t.Errorf("with a result the source is %v high; it should keep to a few lines", compact)
	}
	handle, ok := h.tt.Find("调整原文与结果的高度")
	if !ok {
		t.Fatal("no divider")
	}
	y := handle.Y + 3
	h.tt.Press(400, y)
	h.tt.Move(400, y+60)
	h.tt.Move(400, y+120)
	h.tt.Release(400, y+120)
	h.tt.Frame()
	if got := h.p.sourceBox.H; got < compact+100 {
		t.Errorf("dragging the divider 120 down: source %v → %v", compact, got)
	}
	h.snapshot("split-dragged")
	h.tt.Click("调整原文与结果的高度")
	h.tt.Click("调整原文与结果的高度")
	h.tt.Frame()
	if h.p.sourceHeight != 0 {
		t.Error("a double click did not restore the default")
	}
}

func TestMenuOpensAtOnce(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	h.tt.Click("原文")
	before := h.p.height
	h.sizes = nil
	h.tt.Type("/")
	h.tt.Frame()
	if len(h.sizes) == 0 {
		t.Fatal("the window did not grow for the menu")
	}
	first := h.sizes[0].Height
	h.tt.Advance(time.Second)
	if last := h.sizes[len(h.sizes)-1].Height; first != last || first <= before {
		t.Errorf("the window went %d → %d → %d; the menu should open in one step", before, first, last)
	}
}

// TestMenuWaitsForWindow plays the window: it stays as tall as the app last
// asked, one step behind, as the real window is.
func TestMenuWaitsForWindow(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	h.tt.Click("原文")
	wh := h.p.height
	h.tt.SetSize(820, wh)
	drawn := func() bool { return h.tt.HasText("/ask") && !h.p.menuHidden }
	h.tt.Type("/")
	var seen []bool
	for i := 0; i < 6; i++ {
		seen = append(seen, drawn())
		if n := len(h.sizes); n > 0 {
			wh = h.sizes[n-1].Height // the window catches up
			h.tt.SetSize(820, wh)
		}
		h.tt.Frame()
	}
	_ = seen
	if !drawn() || float32(wh)+1 < h.p.panelH {
		t.Errorf("menu drawn %v with the window %v for a panel %v", drawn(), wh, h.p.panelH)
	}
}

func TestConversationFollowsItsEnd(t *testing.T) {
	long := strings.Repeat("一段回复。\n\n", 60)
	h := newHarness(t, script{pieces: []string{long}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("hello")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Advance(2 * time.Second)
	if !h.p.list.AtEnd() {
		t.Error("a long reply did not keep its end in view")
	}
	if h.tt.HasText("完成") || h.tt.HasText("fast") {
		t.Errorf("the footer still shows the status or model: %q", h.tt.Texts())
	}
	if _, ok := h.tt.Find("复制 Markdown ⌘C"); !ok {
		t.Error("no copy icon")
	}

	h.tt.Scroll(400, 300, 0, -600)
	h.tt.Advance(time.Second)
	if _, ok := h.tt.Find("回到底部"); h.p.list.AtEnd() || !ok {
		t.Error("scrolling up did not stop following or show the way back")
	}
	h.snapshot("scrolled-up")

	h.tt.Click("追问")
	h.tt.Type("再说一遍")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Advance(2 * time.Second)
	if !h.p.list.AtEnd() {
		t.Error("a follow-up did not go to the end")
	}
	h.snapshot("follow-up-end")
}

func TestFitHeight(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"ok"}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("x")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	if h.tt.HasText("适应内容 ⌥⌘0") {
		t.Error("the fit button shows while the panel fits")
	}
	h.p.fixedHeight = 500 // as the user's resize sets it
	h.tt.Frame()
	if !h.tt.HasText("适应内容 ⌥⌘0") {
		t.Fatal("no fit button after a resize")
	}
	h.tt.Key(ui.Cmd|ui.Alt, ui.Key0)
	h.tt.Frame()
	h.tt.Frame()
	if h.p.fixedHeight != 0 || h.tt.HasText("适应内容 ⌥⌘0") || h.p.height >= 500 {
		t.Errorf("after Option-Cmd-0: fixed %d, %d high", h.p.fixedHeight, h.p.height)
	}

	// Held taller than a shorter reply, it shows again, and fits it.
	h.p.sess.Last().Reply = strings.Repeat("line\n\n", 12)
	h.tt.Frame()
	h.settle()
	tall := h.p.height
	h.p.sess.Last().Reply = "ok"
	h.tt.Frame()
	h.tt.Frame()
	if h.p.height != tall || !h.tt.HasText("适应内容 ⌥⌘0") {
		t.Fatalf("a shorter reply: %d high, was %d; fit button %v", h.p.height, tall, h.tt.HasText("适应内容 ⌥⌘0"))
	}
	h.tt.Key(ui.Cmd|ui.Alt, ui.Key0)
	h.tt.Frame()
	h.tt.Frame()
	if h.p.height >= tall || h.tt.HasText("适应内容 ⌥⌘0") {
		t.Errorf("fitting: %d high, was %d", h.p.height, tall)
	}
}

func TestPanelHeightCap(t *testing.T) {
	small := placeOn(mygo.Rectangle{Width: 1440, Height: 875})
	large := placeOn(mygo.Rectangle{Width: 2560, Height: 1415})
	if small.maxHeight != 612 || large.maxHeight != design.PanelMaxHeight {
		t.Errorf("max heights %d and %d", small.maxHeight, large.maxHeight)
	}
}

func TestLateResizesAreNotTheUsers(t *testing.T) {
	release := make(chan struct{})
	client := clientFunc(func(ctx context.Context, req llm.Request, onText func(string)) error {
		<-release
		return nil
	})
	h := newHarness(t, client)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("x")
	h.tt.Key(0, ui.KeyEnter)
	h.tt.Frame()
	h.sizes = nil
	turn := h.p.sess.Last()
	for range 6 {
		turn.Reply += "a line\n\n"
		h.tt.Advance(16 * time.Millisecond)
		h.tt.Frame()
	}
	if len(h.sizes) < 3 {
		t.Fatalf("the panel did not grow: %v", h.sizes)
	}
	// AppKit reports the resizes after the panel asked for later ones.
	for _, r := range h.sizes {
		h.p.resized(r)
	}
	if h.p.fixedHeight != 0 {
		t.Errorf("a late report of the panel's own resize became the user's height, %d", h.p.fixedHeight)
	}
	// The user resizes once the panel has settled.
	for i := range h.p.requested {
		h.p.requested[i].at = h.p.requested[i].at.Add(-2 * askedWithin)
	}
	h.p.resized(mygo.Rectangle{Width: 800, Height: 333})
	if h.p.fixedHeight != 333 {
		t.Errorf("the user's resize to 333 gave %d", h.p.fixedHeight)
	}
	close(release)
	h.drain()
}

func TestFollowUpStaysInALaggingWindow(t *testing.T) {
	release := make(chan struct{})
	client := clientFunc(func(ctx context.Context, req llm.Request, onText func(string)) error {
		<-release
		return nil
	})
	h := newHarness(t, client)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("x")
	h.tt.Key(0, ui.KeyEnter)
	h.tt.Frame()
	h.settle()
	// The window stays as it is while the reply grows: AppKit has not
	// resized it yet.
	h.tt.SetSize(800, h.p.height)
	window := h.p.height
	turn := h.p.sess.Last()
	for range 10 {
		turn.Reply += "## 标题\n一行\n\n"
		h.tt.Advance(16 * time.Millisecond)
		h.tt.Frame()
		f, ok := h.tt.Find("追问")
		if !ok || f.Y+f.H > float32(window) {
			t.Fatalf("the follow-up line is at %v, out of a window %d high", f, window)
		}
	}
	if h.p.least <= window {
		t.Errorf("the panel did not mean to grow: %d", h.p.least)
	}
	close(release)
	h.drain()
}
