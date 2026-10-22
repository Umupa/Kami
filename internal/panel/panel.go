// Package panel is the floating panel: the source, the mode bar, the result
// and its follow-ups. It shows without activating the app, so the app the
// user was in stays in front and gets the keyboard back as the panel hides.
package panel

import (
	"context"
	"errors"
	"log/slog"
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
	// development builds.
	Demo bool
	// Operator is what operator mode works with.
	Operator Operator
	// OnBusy, when set, hears when a request starts and ends, on the main
	// thread, as the menu bar icon shows it.
	OnBusy func(busy bool)
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

	cfg   *config.Config
	modes []mode.Mode

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
	// 0 for the default; split is the drag in progress.
	sourceHeight float32
	sourceBox    ui.Rect
	split        splitDrag
	splitHandle  ui.Handle
	// fixedHeight is the panel's height the user resized it to, 0 to
	// follow its content; requested is the height the app last asked the
	// window for, to tell its own resizes from the user's.
	fixedHeight int
	requested   int

	copiedTurn int
	copiedAt   time.Time

	// place is where the panel shows: its top edge stays put as its height
	// follows the content.
	place  placement
	height int
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
			p.place.x, p.place.top = b.X, b.Y
		}
	})
	p.win.OnResize(func() {
		b := p.win.Bounds()
		if !p.win.IsVisible() || b.Width <= 0 {
			return
		}
		// The panel fills the window across, so a new width needs nothing
		// but to be kept; a height the app did not ask for is the user's.
		p.place.width = b.Width
		if b.Height != p.requested {
			p.fixedHeight, p.height, p.requested = b.Height, b.Height, b.Height
			p.place.maxHeight = max(p.place.maxHeight, b.Height)
		}
	})
	p.win.OnBlur(func() {
		if p.pinned {
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
	p.resize = func(r mygo.Rectangle) {
		p.requested = r.Height
		go func() {
			b := p.win.Bounds()
			if b.Height == r.Height {
				return
			}
			p.win.SetBounds(mygo.Rectangle{X: b.X, Y: b.Y, Width: b.Width, Height: r.Height})
		}()
	}
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
	p.after = func(d time.Duration, fn func()) { time.AfterFunc(d, func() { p.post(fn) }) }
	p.applyConfig(d.Store.Snapshot().Config)
	p.md = markdown.Options{Reveal: &markdown.Reveal{Unit: markdown.ByWord, CatchUp: design.MotionCatchUp, Fade: design.MotionWordFade}}
	return p
}

// Toggle shows the panel, or hides it when it shows.
func (p *Panel) Toggle() {
	if p.win.IsVisible() {
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
	p.requested = p.place.bounds(p.height).Height
	p.win.SetBounds(p.place.bounds(p.height))
	p.win.Show()
	if p.noteMode() {
		p.notes.focusNote = true
	} else {
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
		p.setMode(0)
		p.run()
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
	p.stop()
	p.source, p.follow, p.lastImport = text, "", ""
	p.captureSource = text
	p.sess, p.list, p.notice = nil, newList(), ""
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
	p.cfg, p.modes, p.modeIdx = c, modes, 0
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
}

// run starts a new session with the source in the current mode.
func (p *Panel) run() {
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
	if p.capturedSource() && m.ID == "translate" {
		m.Input = config.InputMaterial
	}
	p.sess = session.New(m, p.source, m.Model)
	p.list = newList()
	p.start()
}

// send asks the follow-up in the follow-up field.
func (p *Panel) send() {
	ask := strings.TrimSpace(p.follow)
	if p.sess == nil || p.sess.Busy() || ask == "" {
		return
	}
	p.sess.FollowUp(ask, p.sess.Last().Model)
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
	p.stop()
	p.sess, p.source, p.follow = nil, "", ""
	p.captureSource = ""
	p.sourceIn.Focus()
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
	model, ok := p.cfg.Model(sess.Turns[turn].Model)
	var client llm.Client
	switch {
	case ok:
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
				if p.sess == sess {
					sess.Turns[turn].Reply += text
				}
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
	defer func() { p.setBusy(p.sess != nil && p.sess.Busy()) }()
	if sess.Operator {
		defer func() {
			if t.Status == session.Done {
				p.finishOperator(t)
			} else {
				t.Reply = operator.Visible(t.Reply)
			}
		}()
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
