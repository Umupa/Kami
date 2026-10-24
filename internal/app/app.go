// Package app runs the menu bar app: its lifecycle, the tray, the watch on
// config.json, and the entry points the panel adds.
package app

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"github.com/egoist/mygo"
	// MyGo's own strings in Simplified Chinese.
	_ "github.com/egoist/mygo/locales/zh-Hans"

	"github.com/Umupa/Kami/internal/capture"
	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/design"
	"github.com/Umupa/Kami/internal/hotkey"
	"github.com/Umupa/Kami/internal/llm"
	"github.com/Umupa/Kami/internal/macos"
	"github.com/Umupa/Kami/internal/mode"
	"github.com/Umupa/Kami/internal/operator"
	"github.com/Umupa/Kami/internal/panel"
	"github.com/Umupa/Kami/internal/selection"
)

// app is the running application. Menu and tray methods are safe from any
// goroutine; everything else is set up before the event loop starts.
type app struct {
	log       *slog.Logger
	configDir string
	store     *config.Store
	tray      *mygo.Tray
	menu      *mygo.Menu
	panel     *panel.Panel
	hotkeys   *hotkey.Manager
	// doubleTaps are the shortcuts that are double taps, each with its
	// action. Main thread only.
	doubleTaps []doubleTap
	tapping    bool
	clipboard  *macos.Pasteboard
	capturer   *capture.Capturer
	// asked holds the permissions asked for in this run: macOS prompts once,
	// so the next ask opens System Settings. mayAskListen is set when
	// Input Monitoring may be asked for unprompted, at a launch with a
	// model; a first run asks for it in the panel's permission step.
	asked        map[string]bool
	mayAskListen bool
	// screenAtLaunch is whether Screen Recording was granted at launch: a
	// grant after takes effect from the next.
	screenAtLaunch   bool
	readingSelection bool
	// undo is config.json before the last operator edit.
	undo []byte
	// statusIcon and brokenIcon are the symbols of the menu's status lines;
	// nil when they could not be installed. conn is what they say of the
	// model, and checks counts the checks so a stale one is dropped. Main
	// thread only.
	statusIcon, brokenIcon, updateIcon *macos.StatusMenuItem
	// updates is what Kami knows of a newer version of itself.
	updates *updates
	conn    connection
	checks  int
	// Dock starts hidden. Only actual setting changes update the native policy.
	dockVisible         bool
	setActivationPolicy func(mygo.ActivationPolicy)
}

// Run starts the app and returns when it quits. A second launch returns at
// once and leaves the running instance in charge.
func Run() error {
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)
	// The app is written in Simplified Chinese only, so MyGo's own controls
	// and menu items speak it too.
	mygo.App.SetLanguages("zh-Hans")

	if err := design.LoadFont(); err != nil {
		return err
	}

	dataDir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return err
	}
	logDir, err := mygo.App.Path(mygo.PathLogs)
	if err != nil {
		logDir = dataDir
	}
	a := &app{log: openLog(logDir, mygo.IsDev()), configDir: dataDir, setActivationPolicy: mygo.App.SetActivationPolicy}
	a.log.Info("launch", "version", mygo.App.Version(), "dev", mygo.IsDev())

	if err := config.Prepare(dataDir); err != nil {
		a.log.Error("prepare config", "err", err)
	}
	a.store = config.NewStore(filepath.Join(dataDir, config.FileName), mode.Check)
	a.logSnapshot(a.store.Snapshot())

	ctx, stop := context.WithCancel(context.Background())
	mygo.App.OnSecondInstance(func([]string, string) {
		if a.panel != nil {
			a.showPanel()
		}
	})
	mygo.App.OnActivate(func(bool) {
		if a.panel != nil {
			a.showPanel()
		}
	})
	mygo.App.WhenReady(func() { a.ready(ctx) })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		stop()
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
	return mygo.App.Run()
}

func (a *app) ready(ctx context.Context) {
	a.updates = newUpdates(a.log, a.showUpdate)
	a.panel = panel.New(panel.Deps{
		Store:  a.store,
		Client: func(config.Model) llm.Client { return llm.HTTP{} },
		Log:    a.log,
		// Canned replies hide the first run, so only on request.
		Demo: mygo.IsDev() && os.Getenv("KAMI_DEMO") == "1",
		Operator: panel.Operator{
			State: a.operatorState,
			Apply: a.applyOps,
			Undo:  a.undoOps,
			Run:   a.runAction,
		},
		AgentPrompt: a.agentPrompt,
		NotesDir: func(c *config.Config) string {
			home, _ := os.UserHomeDir()
			return c.Notes(a.configDir, home)
		},
		Trash:             macos.Trash,
		OnOutcome:         func(err error) { a.requestEnded(err) },
		Permissions:       a.permissionStates,
		RequestPermission: a.requestPermission,
		Relaunch:          mygo.App.Relaunch,
		OnSetup:           func(pending bool) { macos.SetDefaultBool(setupPending, pending) },
		Updates: panel.Updates{
			Offer:   a.updates.Offer,
			State:   a.updates.State,
			Install: a.updates.Install,
			Later:   a.updates.Later,
			Skip:    a.updates.Skip,
			Restart: mygo.App.Relaunch,
			Check:   a.updates.Check,
			Version: mygo.App.Version(),
		},
		OnBusy: func(busy bool) {
			if a.tray != nil {
				a.tray.SetIcon(trayIcon(busy), true)
			}
		},
	})
	a.hotkeys = hotkey.NewManager(a.log)
	a.capturer = capture.New(a.log, a.captured)
	a.screenAtLaunch = macos.ScreenAccess()
	a.log.Info("screen recording access", "allowed", a.screenAtLaunch)
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.capturer.Close() })

	// The menu keeps entry points and the local Dock preference.
	const agentLabel, checkLabel, restartLabel = "复制提示词", "检查更新…", "重新启动 Kami"
	for label, symbol := range map[string]string{
		agentLabel: "doc.on.clipboard", checkLabel: "arrow.down.circle", restartLabel: "arrow.clockwise",
		showDockLabel: "dock.rectangle", hideDockLabel: "dock.rectangle",
	} {
		if cleanup, err := macos.InstallMenuItemSymbol(label, symbol); err != nil {
			a.log.Error("menu icon", "label", label, "err", err)
		} else {
			mygo.App.OnQuit(cleanup)
		}
	}
	// The running version shows beside the item that looks for a newer one.
	if cleanup, err := macos.InstallMenuItemBadge(checkLabel, mygo.App.Version()); err != nil {
		a.log.Error("menu version", "err", err)
	} else {
		mygo.App.OnQuit(cleanup)
	}
	// The status lines are found by the titles they are created with.
	const statusTitle, brokenTitle, updateTitle = "Kami 状态", "Kami 配置", "Kami 更新"
	for _, w := range []struct {
		icon  **macos.StatusMenuItem
		title string
	}{{&a.statusIcon, statusTitle}, {&a.brokenIcon, brokenTitle}, {&a.updateIcon, updateTitle}} {
		if s, err := macos.WatchStatusMenuItem(w.title); err != nil {
			a.log.Error("status menu icon", "err", err)
		} else {
			*w.icon = s
			mygo.App.OnQuit(s.Close)
		}
	}
	a.menu = mygo.NewMenu([]*mygo.MenuItem{
		{ID: "status", Label: statusTitle, Click: func(*mygo.MenuItem, *mygo.Window) { a.showPanel() }},
		{ID: "broken", Label: brokenTitle, Hidden: true, Click: func(*mygo.MenuItem, *mygo.Window) { a.showPanel() }},
		{ID: "update", Label: updateTitle, Hidden: true, Click: func(*mygo.MenuItem, *mygo.Window) {
			a.panel.ShowUpdate()
			a.showPanel()
		}},
		mygo.Separator(),
		{ID: "show", Label: "显示面板", Click: func(*mygo.MenuItem, *mygo.Window) { a.showPanel() }},
		{ID: "capture", Label: "截图识别", Click: func(*mygo.MenuItem, *mygo.Window) { a.startCapture() }},
		mygo.Separator(),
		a.dockMenuItem(),
		// The agent can look into Kami at any time, not only on a first run.
		{Label: agentLabel, Click: func(*mygo.MenuItem, *mygo.Window) { mygo.Clipboard.WriteText(a.agentPrompt()) }},
		mygo.Separator(),
		// Input Monitoring and Screen Recording grants apply from the next
		// launch.
		{Label: checkLabel, Click: func(*mygo.MenuItem, *mygo.Window) {
			a.updates.Check()
			a.panel.ShowUpdate()
			a.showPanel()
		}},
		{Label: restartLabel, Click: func(*mygo.MenuItem, *mygo.Window) { mygo.App.Relaunch() }},
		{Label: "退出 Kami", Role: mygo.RoleQuit},
	})
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:           trayIcon(false),
		IconIsTemplate: true,
		ToolTip:        "Kami",
		Menu:           a.menu,
	})
	if err != nil {
		// Without a tray icon the app has no visible way to quit.
		a.log.Error("tray", "err", err)
		mygo.App.Quit()
		return
	}
	a.tray = tray
	first := a.store.Snapshot().Config.Model == nil
	// A first run asks for nothing before the user has seen the app.
	a.mayAskListen = !first
	a.showSnapshot(a.store.Snapshot())
	a.applyShortcuts(a.store.Snapshot().Config)
	a.applyLaunchAtLogin(a.store.Snapshot().Config)
	a.applyDock(a.store.Snapshot().Config)
	a.store.Subscribe(func(s config.Snapshot) {
		a.logSnapshot(s)
		mygo.RunOnMain(func() {
			a.showSnapshot(s)
			a.applyShortcuts(s.Config)
			a.applyLaunchAtLogin(s.Config)
			a.applyDock(s.Config)
		})
	})
	go a.watchConfig(ctx)
	go a.updates.run(ctx)
	switch {
	case first:
		// Without a model the app has one thing to show: how to get one.
		a.showPanel()
	case macos.DefaultBool(setupPending) && a.panel.ResumeSetup():
		// Granting a permission often restarts Kami midway through.
		a.showPanel()
	}
}

// applyShortcuts registers the global shortcuts of c.
func (a *app) applyShortcuts(c *config.Config) {
	a.log.Info("apply shortcuts", "panel", c.Shortcuts.Panel, "selection", c.Shortcuts.Selection, "capture", c.Shortcuts.Capture)
	bindings := []hotkey.Binding{
		{Name: "panel", Shortcut: c.Shortcuts.Panel, Action: a.togglePanel},
		{Name: "selection", Shortcut: c.Shortcuts.Selection, Action: a.importSelection},
		{Name: "capture", Shortcut: c.Shortcuts.Capture, Action: a.startCapture},
	}
	a.hotkeys.Apply(bindings)
	// The menu shows the combinations; a double tap has no way to be shown
	// there.
	for id, shortcut := range map[string]string{"show": c.Shortcuts.Panel, "capture": c.Shortcuts.Capture} {
		acc := ""
		if sc, err := config.ParseShortcut(shortcut); err == nil && !sc.IsZero() && sc.Double == 0 {
			acc = hotkey.Accelerator(sc)
		}
		a.menu.ItemByID(id).SetAccelerator(acc)
	}
	a.doubleTaps = a.doubleTaps[:0]
	for _, b := range bindings {
		if sc, err := config.ParseShortcut(b.Shortcut); err == nil && sc.Double != 0 {
			a.doubleTaps = append(a.doubleTaps, doubleTap{
				tap:    &hotkey.DoubleTap{Mod: sc.Double, Window: hotkey.DoubleTapWindow},
				action: b.Action,
			})
		}
	}
	if len(a.doubleTaps) > 0 {
		a.watchKeys()
	}
}

// doubleTap is a shortcut that is a double tap, and what it does.
type doubleTap struct {
	tap    *hotkey.DoubleTap
	action func()
}

// applyLaunchAtLogin makes the app a login item, or not, as config.json says.
// Only a packaged app can be one.
func (a *app) applyLaunchAtLogin(c *config.Config) {
	if !mygo.App.IsPackaged() || mygo.App.OpenAtLogin() == c.LaunchAtLogin {
		return
	}
	if err := mygo.App.SetOpenAtLogin(c.LaunchAtLogin); err != nil {
		a.log.Warn("open at login", "err", err)
	}
}

// startCapture hides the panel and freezes the screen for a region.
func (a *app) startCapture() {
	if a.captureBusy() || a.readingSelection {
		return
	}
	a.panel.Hide()
	a.capturer.Start()
}

// captured takes a capture's result: text goes to the panel, no text or a
// failure becomes a notice there, a cancel does nothing.
func (a *app) captured(r capture.Result) {
	switch {
	case r.Canceled:
	case errors.Is(r.Err, macos.ErrNoScreenAccess):
		a.requestPermission(operator.PermScreenRecording)
	case r.Err != nil:
		a.log.Warn("capture", "err", r.Err)
		macos.CaptureFailureFeedback()
	case r.Text == "":
		a.panel.ImportCaptured("")
	default:
		a.panel.ImportCaptured(r.Text)
	}
}

// importSelection reads the selection of the frontmost app and shows the
// panel with it; pressed while the panel shows, it hides the panel. It runs
// on the main thread, where the keyboard layout is read.
func (a *app) importSelection() {
	if a.captureBusy() || a.readingSelection {
		return
	}
	if a.panel.Visible() {
		a.panel.Hide()
		return
	}
	if !macos.AXTrusted() {
		// Without Accessibility there is no selection to read; macOS shows
		// the way to grant it.
		a.requestPermission(operator.PermAccessibility)
		a.panel.Notify("带入选区需要「辅助功能」权限。开关已打开仍无效时，从列表中移除 Kami 再重新添加。")
		return
	}
	if a.clipboard == nil {
		pb, err := macos.GeneralPasteboard()
		if err != nil {
			a.log.Error("pasteboard", "err", err)
			a.showPanel()
			return
		}
		a.clipboard = pb
	}
	r := &selection.Reader{Sys: selection.NewSystem(a.clipboard, macos.KeyCodeFor('c')), Log: a.log}
	a.readingSelection = true
	go func() {
		start := time.Now()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		text := r.Read(ctx)
		a.log.Info("selection read", "found", text != "", "ms", time.Since(start).Milliseconds())
		mygo.RunOnMain(func() { a.readingSelection = false; a.panel.Import(text) })
	}()
}

// watchKeys starts the event tap that sees double taps, once Input
// Monitoring is granted.
func (a *app) watchKeys() {
	if a.tapping {
		return
	}
	err := macos.WatchKeys(func(ev macos.KeyEvent) {
		now, mods := time.Now(), hotkey.ModifiersOf(ev.Flags)
		for _, d := range a.doubleTaps {
			if d.tap.Feed(now, mods, ev.KeyDown) {
				d.action()
			}
		}
	})
	if errors.Is(err, macos.ErrNoListenAccess) && a.mayAskListen && !a.asked[operator.PermInputMonitoring] {
		// macOS prompts once, and applies a grant after a restart.
		a.promptPermission(operator.PermInputMonitoring)
	}
	if err != nil {
		a.log.Warn("watch keys", "err", err)
		return
	}
	a.tapping = true
}

// watchConfig reloads the store whenever config.json changes, until ctx ends.
// A watch that fails or ends, as when the directory is removed, is started
// again after a pause.
func (a *app) watchConfig(ctx context.Context) {
	for ctx.Err() == nil {
		w, err := mygo.WatchFiles(ctx, a.store.Path(), mygo.FileWatchOptions{})
		if err == nil {
			a.store.Reload()
			for {
				if _, err = w.Next(ctx); err != nil {
					break
				}
				a.store.Reload()
			}
			w.Close()
		}
		if ctx.Err() != nil {
			return
		}
		a.log.Warn("config watch stopped", "err", err)
		select {
		case <-ctx.Done():
		case <-time.After(2 * time.Second):
			_ = config.Prepare(a.configDir)
		}
	}
}

// showSnapshot shows a configuration in the menu, and checks the model's
// connection when the model changed. Main thread only.
func (a *app) showSnapshot(s config.Snapshot) {
	if m := s.Config.Model; !reflect.DeepEqual(m, a.conn.model) {
		a.conn = connection{}
		if m != nil {
			copied := *m
			a.conn.model = &copied
			a.check(copied)
		}
	}
	a.showStatus()
}

// setupPending is the user default set while the permission step after the
// first model is unfinished.
const setupPending = "permissionSetupPending"

// checkTimeout bounds the check of a newly configured model.
const checkTimeout = 30 * time.Second

// check sends the model a ping, as `Kami check` does, so the menu can say
// whether it answers before the user sends anything.
func (a *app) check(m config.Model) {
	if m.APIKey == "" && m.Auth != config.AuthNone {
		return
	}
	a.checks++
	n := a.checks
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
		defer cancel()
		_, took, err := llm.Ping(ctx, llm.HTTP{}, m, m.APIKey)
		a.log.Info("model check", "ok", err == nil, "ms", took.Milliseconds())
		mygo.RunOnMain(func() {
			if n == a.checks {
				a.conn.known, a.conn.err = true, err
				a.showStatus()
			}
		})
	}()
}

// requestEnded updates the connection from a request the user sent. Main
// thread only.
func (a *app) requestEnded(err error) {
	if a.conn.model == nil || !tellsConnection(err) {
		return
	}
	a.checks++ // a check still running is older news
	a.conn.known, a.conn.err = true, err
	a.showStatus()
}

// showUpdate shows the newer version in the menu. Main thread only.
func (a *app) showUpdate() {
	if a.menu == nil {
		return
	}
	s, _ := a.updates.State()
	label := a.updates.menuLabel()
	item := a.menu.ItemByID("update")
	item.SetVisible(label != "")
	if label == "" {
		return
	}
	item.SetLabel(label)
	if a.updateIcon != nil {
		symbol, tint := "arrow.down.circle", macos.TintNone
		switch s.Phase {
		case panel.UpdateReady:
			symbol = "arrow.clockwise.circle"
		case panel.UpdateFailed:
			symbol, tint = "exclamationmark.circle.fill", macos.TintOrange
		}
		a.updateIcon.SetSymbol(symbol, tint)
	}
}

// showStatus puts the model and its connection at the top of the menu, with
// a line for a broken config.json. Main thread only.
func (a *app) showStatus() {
	model, broken, tooltip := menuStatus(a.store.Snapshot(), a.conn)
	a.menu.ItemByID("status").SetLabel(model.label)
	if a.statusIcon != nil {
		a.statusIcon.SetSymbol(model.symbol, model.tint)
	}
	item := a.menu.ItemByID("broken")
	item.SetVisible(broken != nil)
	if broken != nil {
		item.SetLabel(broken.label)
		if a.brokenIcon != nil {
			a.brokenIcon.SetSymbol(broken.symbol, broken.tint)
		}
	}
	a.tray.SetToolTip(tooltip)
}

func (a *app) logSnapshot(s config.Snapshot) {
	if s.Err != nil {
		a.log.Warn("config has errors, retaining usable settings", "problems", s.Err.Error())
		return
	}
	a.log.Info("config loaded", "model", s.Config.Model != nil, "modes", len(s.Config.Modes))
}

// All panel entry points wait for the capture that owns the keyboard.
func (a *app) captureBusy() bool { return a.capturer != nil && a.capturer.Busy() }
func (a *app) showPanel() {
	if !a.captureBusy() {
		a.panel.Show()
	}
}
func (a *app) togglePanel() {
	if !a.captureBusy() && !a.readingSelection {
		a.panel.Toggle()
	}
}
