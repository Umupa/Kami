// Package panel is the floating panel: the source, the mode bar, the result
// and its follow-ups. It shows without activating the app, so the app the
// user was in stays in front and gets the keyboard back as the panel hides.
package panel

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/design"
	"github.com/Umupa/Kami/internal/llm"
	"github.com/Umupa/Kami/internal/mode"
	"github.com/Umupa/Kami/internal/operator"
	"github.com/Umupa/Kami/internal/segment"
	"github.com/Umupa/Kami/internal/session"
)

// Deps are what the panel works with.
type Deps struct {
	Store *config.Store
	// Client returns the client of a model's format.
	Client func(config.Model) llm.Client
	Log    *slog.Logger
	// Demo answers with a canned reply when no model is configured, for
	// development builds that ask for it.
	Demo bool
	// Operator is what operator mode works with.
	Operator Operator
	// AgentPrompt returns the prompt that has a coding agent set up, fix or
	// adjust the model.
	AgentPrompt func() string
	// OnBusy, when set, hears when a request starts and ends, on the main
	// thread, as the menu bar icon shows it.
	OnBusy func(busy bool)
	// OnOutcome, when set, hears how each request to the model ended, on
	// the main thread: nil when it was answered. Stops and Kami's own
	// replies are not requests.
	OnOutcome func(err error)
	// Permissions reads the system permissions; RequestPermission asks for
	// one, with macOS's prompt or, once it was shown, System Settings;
	// Relaunch starts Kami again, as some grants need. Main thread.
	Permissions       func() []operator.Permission
	RequestPermission func(id string)
	Relaunch          func()
	// OnSetup, when set, hears the permission step start and end, so that
	// the app resumes it after a restart (ResumeSetup).
	OnSetup func(pending bool)
	// Updates is what the panel does about a newer version of Kami.
	Updates Updates
	// NotesDir is where a configuration keeps the notes, and Trash moves a
	// deleted note's file to the Trash.
	NotesDir func(*config.Config) string
	Trash    func(path string) error
}

// Panel is the panel window and its state. Everything but Show, Hide and
// Toggle runs on the main thread.
type Panel struct {
	deps Deps
	win  *mygo.Window
	// traceLayoutEnabled opts into geometry-only diagnostics for flicker.
	traceLayoutEnabled bool
	// post runs a function on the main thread and draws a frame: the
	// window's Update, or a test's queue.
	post func(func())
	// resize sets the window's bounds after the view has been built.
	resize func(mygo.Rectangle)
	// writeClipboard copies text, and writeRich HTML with its plain text:
	// the system clipboard, or a test's.
	writeClipboard func(string)
	writeRich      func(html, text string)
	// background runs file work off the main thread, after runs a function
	// on the main thread a while later, and openURL opens a link in the
	// browser: the system's, or a test's.
	background func(func())
	after      func(time.Duration, func())
	openURL    func(string)

	custom map[string]*design.Palette
	cfg    *config.Config
	modes  []mode.Mode

	source string
	// captureSource keeps unedited OCR material out of the command parser.
	captureSource string
	// lastImport is the selection last brought in, so that importing it
	// again shows the panel as the user left it.
	lastImport string
	notice     string

	// operatorSince is when the source turned to operator mode, for the
	// sheen; sparkleAt when the latest operator request was sent.
	operatorSince  time.Time
	sparkleAt      time.Time
	sparklePending bool
	sparkles       []sparkle
	follow         string
	// modeIdx is the current mode, len(modes) for notes.
	modeIdx int
	notes   notes
	sess    *session.Session
	cancel  context.CancelFunc
	// said is the text of the app's own reply last shown (greet).
	said string
	// inSetup is set during the permission step, and heldOpen while the
	// panel stays up as the user works elsewhere: in a coding agent, or in
	// System Settings.
	inSetup  bool
	heldOpen bool
	// updating is set while the update screen shows; updateIdx is its
	// chosen item, focusUpdate asks for its keyboard focus, and
	// updateConfirmed is set once the user asked for the restart while
	// the panel is up. The panel grows to show the notes, whose scroll
	// range updateScroll measures, and goes back to leastBeforeUpdate as
	// the screen closes.
	updating          bool
	updateIdx         int
	focusUpdate       bool
	updateConfirmed   bool
	updateMD          markdown.Options
	updateScroll      ui.ScrollState
	leastBeforeUpdate int
	// results holds the other modes' sessions, by mode id.
	results map[string]result

	md    markdown.Options
	mdFor mdKey           // what md.Theme was made for
	pal   *design.Palette // the palette of the last frame
	sz    design.Sizes    // the type sizes of the last frame

	// pinned keeps the panel when another app takes the keyboard.
	pinned bool
	// list is the conversation, which follows its end as a coding agent's
	// transcript does: a reply streaming in stays in view until the user
	// scrolls up, and scrolling back down follows again.
	list     conversationList
	sourceIn ui.Handle
	followIn ui.Handle
	// Whether the inputs had a selection in the last frame: Cmd-C copies it
	// when they do, and the latest reply when they don't.
	sourceSelected, followSelected bool

	// menuIdx is the chosen row of the menu under the source; menuClosed
	// the source the menu was dismissed for.
	menuIdx    int
	menuClosed string
	// menuChosenFor is the source for which an arrow key chose a menu row.
	// Without that choice, Enter on bare /operator still opens the guide.
	menuChosenFor string
	// menuRows is how many rows the menu has; menuGrew counts the frames
	// since it opened or grew, while it waits for the window (menuWaiting).
	menuRows, menuGrew int
	menuHidden         bool
	// panelH is the panel's height as the last frame laid it out.
	panelH float32
	// caretToEnd puts the caret after the source's text in the next frame,
	// as after a command was picked.
	caretToEnd bool

	// sourceHeight is the source's height the user dragged the divider to,
	// 0 for the default; split is the drag in progress. underSource is
	// the height of what keeps its own under the source, as the last frame
	// drew it, with sourceH and resultH: the divider, the bars, the menu.
	sourceHeight float32
	sourceBox    ui.Rect
	underSource  float32
	sourceH      float32
	resultH      float32
	// The rows painted this frame measure short replies without counting
	// spare viewport space. A partially visible list uses its scroll range.
	resultRowsH float32
	resultRows  int
	split       splitDrag
	splitHandle ui.Handle
	// fixedHeight is the panel's height the user resized it to, 0 to
	// follow its content; requested are the heights the app asked the
	// window for lately, to tell its own resizes from the user's: AppKit
	// reports them later, while the panel growing has asked for others since.
	fixedHeight int
	requested   []asked

	copiedTurn int
	copiedAt   time.Time

	// place is where the panel shows: its top edge stays put as its height
	// follows the content.
	place  placement
	height int
	// least is the height the panel keeps until a new session: it grows
	// with its content and does not shrink back (fit). natural is the
	// content's own height, without the filler that holds the rest.
	least, natural int
	// noteH is the height of the note's own content, which notes mode
	// takes instead of least.
	noteH   int
	fillerH float32
}

// resized takes the window's new bounds. The panel fills the window
// across, so a new width needs nothing but to be kept; a height the app did
// not ask for is the user's.
func (p *Panel) resized(b mygo.Rectangle) {
	if b.Width <= 0 {
		return
	}
	p.place.width = b.Width
	own := p.asked(b.Height)
	p.traceLayout("resize-callback", "bounds", b, "own", own)
	if !own {
		p.fixedHeight, p.height = b.Height, b.Height
		p.ask(b.Height)
		p.place.maxHeight = max(p.place.maxHeight, b.Height)
	}
}

// asked is a height the app asked the window for, and when.
type asked struct {
	height int
	at     time.Time
}

// askedWithin is how long a resize the app asked for may take to be
// reported.
const askedWithin = time.Second

// ask records a height the app asks the window for.
func (p *Panel) ask(h int) {
	now := time.Now()
	keep := p.requested[:0]
	for _, a := range p.requested {
		if now.Sub(a.at) < askedWithin {
			keep = append(keep, a)
		}
	}
	p.requested = append(keep, asked{h, now})
}

// asked reports whether the app may have asked for height h: it is between
// the heights asked for lately, which a growing reply passes through, or the last
// one asked for.
func (p *Panel) asked(h int) bool {
	if len(p.requested) == 0 {
		return false
	}
	last := p.requested[len(p.requested)-1]
	if h == last.height {
		return true
	}
	lo, hi := last.height, last.height
	for _, a := range p.requested {
		if time.Since(a.at) < askedWithin {
			lo, hi = min(lo, a.height), max(hi, a.height)
		}
	}
	return h >= lo && h <= hi
}

// initialHeight is about the height of the empty panel: the source's line
// and the mode bar.
const initialHeight = 110

// New creates the hidden panel window. Call it once the app is ready.
func New(d Deps) *Panel {
	p := newPanel(d)
	p.win = mygo.NewWindow(mygo.WindowOptions{
		Title:             "Kami",
		Panel:             true,
		Hidden:            true,
		Frameless:         true,
		Transparent:       true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		Width:             design.PanelWidth,
		Height:            initialHeight,
		Vibrancy:          mygo.VibrancyHUD,
		Content:           ui.View(p.view),
	})
	// Escape and Cmd-W close a panel; hiding keeps its state and shows it
	// at once the next time.
	// Quitting must still close it, or the app could not quit.
	quitting := false
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		quitting = true
		p.saveNote()
	})
	p.win.OnClose(func(e *mygo.CloseEvent) {
		if quitting {
			return
		}
		e.PreventDefault()
		p.win.Hide()
	})
	// The user moves the panel by its mode bar and resizes it by its edges;
	// the place and size they leave it at are kept.
	p.win.OnMove(func() {
		if b := p.win.Bounds(); p.win.IsVisible() {
			p.traceLayout("move-callback", "bounds", b)
			p.place.x, p.place.top = b.X, b.Y
		}
	})
	p.win.OnResize(func() {
		if p.win.IsVisible() {
			p.resized(p.win.Bounds())
		}
	})
	p.win.OnBlur(func() {
		if p.pinned || p.heldOpen {
			return
		}
		d.Log.Debug("panel blur")
		p.win.Hide()
	})
	p.win.OnFocus(func() { d.Log.Debug("panel focus") })
	p.win.OnShow(func() { d.Log.Debug("panel shown", "bounds", p.win.Bounds()) })
	p.win.OnHide(func() {
		d.Log.Debug("panel hidden")
		p.leaveNote()
	})
	p.post = p.win.Update
	// Resizing re-enters layout, so it waits until the view has been built.
	// The window is resized as soon as the frame that measured the content
	// is drawn: from the paint, the call waits for the main thread, which
	// AppKit then redraws at the new size within the same pass. Only the
	// height changes: where the window is now, which the user may be
	// moving it from, and its width stay.
	sizer := &heightSetter{apply: func(h int) {
		b := p.win.Bounds()
		if b.Height == h {
			return
		}
		p.traceLayout("native-apply", "before", b, "height", h)
		p.win.SetBounds(mygo.Rectangle{X: b.X, Y: b.Y, Width: b.Width, Height: h})
		if p.traceLayoutEnabled {
			p.traceLayout("native-applied", "bounds", p.win.Bounds())
		}
	}}
	p.resize = func(r mygo.Rectangle) { sizer.set(r.Height) }
	d.Store.Subscribe(func(s config.Snapshot) {
		p.post(func() { p.applyConfig(s.Config) })
	})
	return p
}

// newPanel returns a panel without a window, as tests drive it.
func newPanel(d Deps) *Panel {
	p := &Panel{deps: d, list: newList(), post: func(fn func()) { fn() }, resize: func(mygo.Rectangle) {}, writeClipboard: mygo.Clipboard.WriteText, writeRich: writeRich,
		background: func(fn func()) { go fn() },
		openURL:    func(url string) { _ = mygo.Shell.OpenExternal(url) }}
	p.traceLayoutEnabled = os.Getenv("KAMI_TRACE_LAYOUT") == "1"
	p.after = func(d time.Duration, fn func()) { time.AfterFunc(d, func() { p.post(fn) }) }
	p.applyConfig(d.Store.Snapshot().Config)
	p.md = markdown.Options{Reveal: &markdown.Reveal{Unit: markdown.ByWord, CatchUp: design.MotionCatchUp, Fade: design.MotionWordFade}}
	return p
}

// Toggle shows the panel, or hides it when it shows.
func (p *Panel) Toggle() {
	// A panel left up behind other windows comes forward instead.
	if p.win.IsVisible() && p.win.IsFocused() {
		p.win.Hide()
		return
	}
	p.Show()
}

// Show shows the panel on the display with the pointer, with the source
// focused and its selection kept.
func (p *Panel) Show() {
	d := mygo.Screen.DisplayNearestPoint(mygo.Screen.CursorScreenPoint())
	if !p.place.within(d.WorkArea) {
		// Where the user put the panel stays, on its display; elsewhere it
		// shows centered again, as wide as they made it.
		width := p.place.width
		p.place = placeOn(d.WorkArea)
		if width > 0 {
			p.place.width = width
			p.place.x = d.WorkArea.X + (d.WorkArea.Width-width)/2
		}
	}
	if p.height == 0 {
		// Until the view has measured the panel, the window takes the
		// height of an empty one.
		p.height = initialHeight
	}
	p.ask(p.place.bounds(p.height).Height)
	p.win.SetBounds(p.place.bounds(p.height))
	p.greet()
	// A restart asked for and left behind waits for the next ask.
	p.updateConfirmed = false
	p.offerUpdate()
	p.win.Show()
	// Kami is not the active app, and makeKeyAndOrderFront alone may leave
	// the panel behind the active app's windows on that display;
	// orderFrontRegardless puts it at the front of its level all the same.
	p.win.ShowInactive()
	switch {
	case p.noteMode():
		p.notes.focusNote, p.notes.listOpen = true, false
	case p.operatorSession():
		p.followIn.Focus()
	default:
		p.sourceIn.Focus()
	}
	p.deps.Log.Info("panel show", "work", d.WorkArea, "want", p.place.bounds(p.height),
		"got", p.win.Bounds(), "visible", p.win.IsVisible())
}

// writeRich puts HTML on the clipboard with its plain text, so that apps
// that take rich text keep tables and lists, and the rest get clean text.
func writeRich(html, text string) {
	_ = mygo.Clipboard.Write(transfer.New(transfer.NewItem(
		transfer.Bytes(transfer.HTML, []byte(html)),
		transfer.Bytes(transfer.Text, []byte(text)),
	)))
}

// Visible reports whether the panel shows.
func (p *Panel) Visible() bool { return p.win.IsVisible() }

// Import shows the panel with text selected in another app. New text starts
// a session in the default mode at once; the same text as last time, or none,
// shows the panel as it was.
func (p *Panel) Import(text string) {
	switch {
	case text == "":
		p.lastImport = ""
	case text != p.lastImport:
		p.lastImport = text
		p.captureSource = ""
		p.source = text
		p.clearResults()
		p.least = 0
		p.setMode(0)
		// With no model the text waits in the source for the first one.
		if !p.unconfigured() {
			p.run()
		}
	}
	p.Show()
}

// ImportCaptured shows the panel with text recognized in a screenshot and
// always translates it, superseding a running request.
func (p *Panel) ImportCaptured(text string) {
	p.importCaptured(text)
	p.Show()
}

// importCaptured is the window-independent part of importing OCR output.
func (p *Panel) importCaptured(text string) {
	p.clearResults()
	p.least = 0
	p.source, p.follow, p.lastImport = text, "", ""
	p.captureSource = text
	p.notice = ""
	p.setMode(0)
	index := -1
	for i, m := range p.modes {
		if m.ID == "translate" {
			index = i
			break
		}
	}
	if index < 0 {
		c := *p.cfg
		c.Modes = []config.Mode{{ID: "translate"}}
		modes, _ := mode.Resolve(&c)
		index = len(p.modes)
		p.modes = append(p.modes, modes[0])
	}
	p.setMode(index)
	if strings.TrimSpace(text) == "" {
		p.source = ""
		p.notice = "截图里没有识别到文字"
		p.setBusy(false)
		return
	}
	if p.unconfigured() {
		// The text waits in the source for the first model.
		p.greet()
		return
	}
	// OCR is material, including text that happens to start with a slash
	// command. It must never be interpreted as an Operator instruction.
	p.runMaterial()
}

// Notify shows the panel with a short notice in place of a result, as when
// a screenshot had no text. Running or editing clears it.
func (p *Panel) Notify(notice string) {
	p.notice = notice
	p.Show()
}

// Hide hides the panel. A request in progress goes on.
func (p *Panel) Hide() { p.win.Hide() }

func (p *Panel) applyConfig(c *config.Config) {
	modes, err := mode.Resolve(c)
	if err != nil {
		// The store checks modes before accepting a config, so this is a
		// config the store never had.
		p.deps.Log.Error("resolve modes", "err", err)
		return
	}
	var current string
	if p.modeIdx < len(p.modes) {
		current = p.modes[p.modeIdx].ID
	}
	inNotes := p.cfg != nil && p.noteMode()
	arrived := p.unconfigured() && c.Model != nil
	p.cfg, p.modes, p.modeIdx = c, modes, 0
	p.custom = make(map[string]*design.Palette, len(c.Themes))
	for _, t := range c.Themes {
		p.custom[t.ID] = design.Derive(design.Themes[t.Base], t.Colors)
	}
	for i, m := range modes {
		if m.ID == current {
			p.modeIdx = i
		}
	}
	if inNotes {
		p.modeIdx = len(modes)
	}
	if p.deps.NotesDir != nil {
		p.setNotesDir(p.deps.NotesDir(c))
	}
	// The panel greets as it shows (Show); here it only keeps what it said
	// up to date.
	switch {
	case arrived:
		p.connected()
	case p.greeting():
		p.greet()
	}
}

// run starts a new session with the source in the current mode. With no
// model to send it to, it copies the prompt that configures one, and the
// source waits.
func (p *Panel) run() {
	if p.unconfigured() {
		if !themeTyped(p.source) || p.capturedSource() {
			p.copyTurn(-1)
		}
		return
	}
	if p.welcoming() && p.permissionStep() {
		return
	}
	if p.inSetup {
		// Using Kami ends the step, granted or not.
		p.endSetup()
	}
	if request, ok := operator.Leading(p.source); ok && !p.capturedSource() {
		p.runOperator(request)
		return
	}
	if strings.TrimSpace(p.source) == "" || len(p.modes) == 0 || p.noteMode() || (!p.capturedSource() && themeTyped(p.source)) {
		return
	}
	p.runMaterial()
}

// runMaterial starts a session without interpreting source text as commands.
func (p *Panel) runMaterial() {
	p.stop()
	p.notice = ""
	m := p.modes[p.modeIdx]
	p.dropResult(m.ID)
	if p.capturedSource() && m.ID == "translate" {
		m.Input = config.InputMaterial
	}
	p.sess = session.New(m, p.source)
	p.list = newList()
	p.start()
}

// send asks the follow-up in the follow-up field.
func (p *Panel) send() {
	ask := strings.TrimSpace(p.follow)
	if p.sess == nil || p.sess.Busy() || ask == "" {
		return
	}
	p.sess.FollowUp(ask)
	p.follow = ""
	// A follow-up goes to the end, wherever the user had scrolled.
	p.list.ScrollToEnd()
	p.start()
}

// retry runs the latest turn again.
func (p *Panel) retry() {
	if p.sess == nil || p.sess.Busy() {
		return
	}
	p.sess.Retry()
	p.start()
}

// stop ends the request in progress; what came so far stays.
func (p *Panel) stop() {
	if p.cancel != nil {
		p.cancel()
		p.cancel = nil
	}
}

// reset starts over with an empty source.
func (p *Panel) reset() {
	p.clearResults()
	p.source, p.follow = "", ""
	p.least = 0
	p.captureSource = ""
	p.sourceIn.Focus()
	p.greet()
}

func (p *Panel) setBusy(busy bool) {
	if p.deps.OnBusy != nil {
		p.deps.OnBusy(busy)
	}
}

// errNoModel is a request without a configured model.
var errNoModel = &llm.Error{Kind: llm.KindConfig, Detail: "no model configured"}

// start streams the latest turn of the session on a goroutine.
func (p *Panel) start() {
	sess, turn := p.sess, len(p.sess.Turns)-1
	sess.Turns[turn].Status = session.Streaming
	var model config.Model
	var client llm.Client
	switch {
	case p.cfg.Model != nil:
		model = *p.cfg.Model
		client = p.deps.Client(model)
	case p.deps.Demo:
		client = llm.Demo{}
	default:
		p.finish(sess, turn, errNoModel)
		return
	}
	system := mode.System(sess.Mode, p.cfg)
	if sess.Operator {
		system = operator.System(p.deps.Operator.State())
	}
	req := llm.Request{Model: model, Key: model.APIKey, System: system, Messages: sess.Messages()}
	material := sess.Mode.Input == config.InputMaterial
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.setBusy(true)
	go func() {
		defer cancel()
		replied := false
		emit := func(text string) {
			replied = true
			p.post(func() {
				sess.Turns[turn].Reply += text
			})
		}
		send := func(ctx context.Context, msgs []llm.Message, onText func(string)) error {
			r := req
			r.Messages = msgs
			return client.Stream(ctx, r, onText)
		}
		msgs, dropped := fitContext(req, model)
		err := send(ctx, msgs, emit)
		parts := 0
		if session.TooLong(err) && !replied {
			switch {
			case len(req.Messages) > 3:
				// The model's limit is lower than configured, or unknown:
				// keep the source turn, so that the request can still refer
				// to its result, and the request itself.
				all := req.Messages
				msgs = []llm.Message{all[0], all[1], all[len(all)-1]}
				dropped = (len(all) - 3) / 2
				err = send(ctx, msgs, emit)
			case len(req.Messages) == 1 && material:
				parts, err = session.StreamParts(ctx, msgs[0].Content, segment.Split,
					func(ctx context.Context, part string, onText func(string)) error {
						return send(ctx, []llm.Message{{Role: llm.RoleUser, Content: part}}, onText)
					}, emit)
			}
		}
		p.post(func() {
			t := &sess.Turns[turn]
			t.Dropped, t.Parts = dropped, parts
			p.finish(sess, turn, err)
		})
	}()
}

// fitContext leaves out earlier turns that do not fit the model's context
// window, keeping room for the reply.
func fitContext(req llm.Request, m config.Model) ([]llm.Message, int) {
	if m.ContextWindow <= 0 {
		return req.Messages, 0
	}
	reserve := min(8192, m.ContextWindow/4)
	return session.Trim(req.Messages, m.ContextWindow-reserve-session.EstimateTokens(req.System))
}

// finish records how a turn ended.
func (p *Panel) finish(sess *session.Session, turn int, err error) {
	t := &sess.Turns[turn]
	defer func() { p.setBusy(p.busy()) }()
	if sess.Operator {
		defer func() {
			if t.Status == session.Done {
				p.finishOperator(t)
			} else {
				t.Reply = operator.Visible(t.Reply)
			}
		}()
	}
	if p.deps.OnOutcome != nil && !t.Local && !errors.Is(err, context.Canceled) && err != errNoModel {
		p.deps.OnOutcome(err)
	}
	switch {
	case err == nil:
		t.Status = session.Done
	case errors.Is(err, context.Canceled):
		t.Status = session.Stopped
	default:
		t.Status, t.Err = session.Failed, err
		var le *llm.Error
		if errors.As(err, &le) {
			p.deps.Log.Warn("request failed", "kind", le.Kind.String(), "status", le.Status)
		}
	}
}

// conversationList keeps the measured scroll extent with the list so both
// reset together when a new conversation starts.
type conversationList struct {
	ui.ListState
	scroll ui.ScrollState
}

// newList returns the state of a new conversation's list.
func newList() conversationList {
	return conversationList{}
}

func (p *Panel) capturedSource() bool { return p.captureSource != "" && p.source == p.captureSource }

// paletteFor resolves configured and preview themes from cached palettes.
func (p *Panel) paletteFor(name string, dark bool) *design.Palette {
	if pal := p.custom[name]; pal != nil {
		return pal
	}
	return design.For(name, dark)
}
