// Package panel is the floating panel: the source, the mode bar, the result
// and its follow-ups. It shows without activating the app, so the app the
// user was in stays in front and gets the keyboard back as the panel hides.
package panel

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/design"
	"github.com/Umupa/cida-next/internal/keyring"
	"github.com/Umupa/cida-next/internal/llm"
	"github.com/Umupa/cida-next/internal/mode"
	"github.com/Umupa/cida-next/internal/session"
)

// Deps are what the panel works with.
type Deps struct {
	Store   *config.Store
	Keyring keyring.Keyring
	// Client returns the client of a model's format.
	Client func(config.Model) llm.Client
	Log    *slog.Logger
	// Demo answers with a canned reply when no model is configured, for
	// development builds.
	Demo bool
	// OnBusy, when set, hears when a request starts and ends, on the main
	// thread, as the menu bar icon shows it.
	OnBusy func(busy bool)
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

	cfg   *config.Config
	modes []mode.Mode

	source string
	// lastImport is the selection last brought in, so that importing it
	// again shows the panel as the user left it.
	lastImport string
	notice     string
	follow     string
	modeIdx    int
	sess       *session.Session
	cancel     context.CancelFunc

	md       markdown.Options
	result   ui.ScrollState
	sourceIn ui.Handle
	followIn ui.Handle
	// Whether the inputs had a selection in the last frame: Cmd-C copies it
	// when they do, and the latest reply when they don't.
	sourceSelected, followSelected bool

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
		Title:             "cida-next",
		Panel:             true,
		Hidden:            true,
		Frameless:         true,
		Transparent:       true,
		DisableResize:     true,
		DisableMinimize:   true,
		DisableMaximize:   true,
		DisableFullScreen: true,
		Width:             design.PanelWidth,
		Height:            initialHeight,
		Content:           ui.View(p.view),
	})
	// Escape and Cmd-W close a panel; hiding keeps its state and shows it
	// at once the next time.
	// Quitting must still close it, or the app could not quit.
	quitting := false
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { quitting = true })
	p.win.OnClose(func(e *mygo.CloseEvent) {
		if quitting {
			return
		}
		e.PreventDefault()
		p.win.Hide()
	})
	p.win.OnBlur(func() {
		d.Log.Debug("panel blur")
		p.win.Hide()
	})
	p.win.OnFocus(func() { d.Log.Debug("panel focus") })
	p.win.OnShow(func() { d.Log.Debug("panel shown", "bounds", p.win.Bounds()) })
	p.win.OnHide(func() { d.Log.Debug("panel hidden") })
	p.post = p.win.Update
	// Resizing re-enters layout, so it waits until the view has been built.
	p.resize = func(r mygo.Rectangle) {
		go func() {
			p.win.SetBounds(r)
			d.Log.Debug("panel resize", "want", r, "got", p.win.Bounds())
		}()
	}
	d.Store.Subscribe(func(s config.Snapshot) {
		p.post(func() { p.applyConfig(s.Config) })
	})
	return p
}

// newPanel returns a panel without a window, as tests drive it.
func newPanel(d Deps) *Panel {
	p := &Panel{deps: d, post: func(fn func()) { fn() }, resize: func(mygo.Rectangle) {}, writeClipboard: mygo.Clipboard.WriteText, writeRich: writeRich}
	p.applyConfig(d.Store.Snapshot().Config)
	p.md = markdown.Options{Theme: &markdown.Theme{FontSize: design.FontResultCJK}}
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
	p.place = placeOn(d.WorkArea)
	if p.height == 0 {
		// Until the view has measured the panel, the window takes the
		// height of an empty one.
		p.height = initialHeight
	}
	p.win.SetBounds(p.place.bounds(p.height))
	p.win.Show()
	p.sourceIn.Focus()
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
// shows the panel as it was (Cida's panel.md §一).
func (p *Panel) Import(text string) {
	switch {
	case text == "":
		p.lastImport = ""
	case text != p.lastImport:
		p.lastImport = text
		p.source = text
		p.modeIdx = 0
		p.run()
	}
	p.Show()
}

// ImportCaptured shows the panel with text recognized in a screenshot and
// runs the default mode on it.
func (p *Panel) ImportCaptured(text string) {
	p.notice = ""
	p.source = text
	p.modeIdx = 0
	p.run()
	p.Show()
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
	p.cfg, p.modes, p.modeIdx = c, modes, 0
	for i, m := range modes {
		if m.ID == current {
			p.modeIdx = i
		}
	}
}

// run starts a new session with the source in the current mode.
func (p *Panel) run() {
	if strings.TrimSpace(p.source) == "" || len(p.modes) == 0 {
		return
	}
	p.stop()
	p.notice = ""
	m := p.modes[p.modeIdx]
	p.sess = session.New(m, p.source, m.Model)
	p.result = ui.ScrollState{}
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
	req := llm.Request{Model: model, System: mode.System(sess.Mode, p.cfg), Messages: sess.Messages()}
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.setBusy(true)
	go func() {
		defer cancel()
		if ok && model.Auth != config.AuthNone {
			// The keychain may ask the user, so it is read off the main thread.
			key, err := p.deps.Keyring.Get(model.Name)
			if err != nil && !errors.Is(err, keyring.ErrNotFound) {
				p.deps.Log.Warn("read key", "model", model.Name, "err", err)
			}
			req.Key = key
		}
		err := client.Stream(ctx, req, func(text string) {
			p.post(func() {
				if p.sess == sess {
					sess.Turns[turn].Reply += text
				}
			})
		})
		p.post(func() { p.finish(sess, turn, err) })
	}()
}

// finish records how a turn ended.
func (p *Panel) finish(sess *session.Session, turn int, err error) {
	t := &sess.Turns[turn]
	defer func() { p.setBusy(p.sess != nil && p.sess.Busy()) }()
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
