// Package app runs the menu bar app: its lifecycle, the tray, the watch on
// config.json, and the entry points the panel adds.
package app

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	// MyGo's own strings in Simplified Chinese.
	_ "github.com/egoist/mygo/locales/zh-Hans"

	"github.com/Umupa/Gloss/internal/capture"
	"github.com/Umupa/Gloss/internal/config"
	"github.com/Umupa/Gloss/internal/design"
	"github.com/Umupa/Gloss/internal/hotkey"
	"github.com/Umupa/Gloss/internal/llm"
	"github.com/Umupa/Gloss/internal/macos"
	"github.com/Umupa/Gloss/internal/mode"
	"github.com/Umupa/Gloss/internal/panel"
	"github.com/Umupa/Gloss/internal/selection"
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
	// askedListen is set once Input Monitoring has been asked for.
	askedListen      bool
	readingSelection bool
	// undo is config.json before the last operator edit.
	undo []byte
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
	a := &app{log: openLog(logDir, mygo.IsDev()), configDir: dataDir}
	a.log.Info("launch", "version", mygo.App.Version(), "dev", mygo.IsDev())

	if err := config.Prepare(dataDir); err != nil {
		a.log.Error("prepare config", "err", err)
	}
	a.store = config.NewStore(filepath.Join(dataDir, config.FileName), mode.Check)
	a.migrateKeys()
	a.logSnapshot(a.store.Snapshot())

	ctx, stop := context.WithCancel(context.Background())
	mygo.App.OnSecondInstance(func([]string, string) {
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
	a.panel = panel.New(panel.Deps{
		Store:  a.store,
		Client: func(config.Model) llm.Client { return llm.HTTP{} },
		Log:    a.log,
		Demo:   mygo.IsDev(),
		Operator: panel.Operator{
			State: a.operatorState,
			Apply: a.applyOps,
			Undo:  a.undoOps,
			Run:   a.runAction,
		},
		NotesDir: func(c *config.Config) string {
			home, _ := os.UserHomeDir()
			return c.Notes(a.configDir, home)
		},
		Trash: macos.Trash,
		OnBusy: func(busy bool) {
			if a.tray != nil {
				a.tray.SetIcon(trayIcon(busy), true)
			}
		},
	})
	a.hotkeys = hotkey.NewManager(a.log)
	a.capturer = capture.New(a.log, a.captured)
	a.log.Info("screen recording access", "allowed", macos.ScreenAccess())
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) { a.capturer.Close() })

	// Everything else is in the panel: /operator shows and changes settings.
	a.menu = mygo.NewMenu([]*mygo.MenuItem{
		{ID: "config-error", Hidden: true, Click: func(*mygo.MenuItem, *mygo.Window) { a.showPanel() }},
		{ID: "config-error-sep", Type: mygo.MenuItemSeparator, Hidden: true},
		{Label: "显示面板", Click: func(*mygo.MenuItem, *mygo.Window) { a.showPanel() }},
		{Label: "截图识别", Click: func(*mygo.MenuItem, *mygo.Window) { a.startCapture() }},
		mygo.Separator(),
		{Label: "退出 Gloss", Role: mygo.RoleQuit},
	})
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:           trayIcon(false),
		IconIsTemplate: true,
		ToolTip:        "Gloss",
		Menu:           a.menu,
	})
	if err != nil {
		// Without a tray icon the app has no visible way to quit.
		a.log.Error("tray", "err", err)
		mygo.App.Quit()
		return
	}
	a.tray = tray
	a.showSnapshot(a.store.Snapshot())
	a.applyShortcuts(a.store.Snapshot().Config)
	a.applyLaunchAtLogin(a.store.Snapshot().Config)
	a.store.Subscribe(func(s config.Snapshot) {
		a.logSnapshot(s)
		a.showSnapshot(s)
		mygo.RunOnMain(func() {
			a.applyShortcuts(s.Config)
			a.applyLaunchAtLogin(s.Config)
		})
	})
	go a.watchConfig(ctx)
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
		a.log.Info("capture permission requested")
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
		macos.RequestAXTrust()
		a.panel.Notify("带入选中的文字需要「辅助功能」权限：在系统设置里允许 Gloss。")
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
	r := &selection.Reader{Sys: selection.NewSystem(a.clipboard, macos.KeyCodeFor('c'))}
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
	if errors.Is(err, macos.ErrNoListenAccess) && !a.askedListen {
		// macOS prompts once, and applies a grant after a restart.
		a.askedListen = true
		macos.RequestListenAccess()
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

// showSnapshot puts what is wrong with config.json at the top of the menu.
func (a *app) showSnapshot(s config.Snapshot) {
	item := a.menu.ItemByID("config-error")
	sep := a.menu.ItemByID("config-error-sep")
	if s.Err == nil {
		item.SetVisible(false)
		sep.SetVisible(false)
		a.tray.SetToolTip("Gloss")
		return
	}
	label := "配置有误，沿用上一份配置"
	var ce *config.Error
	if errors.As(s.Err, &ce) && len(ce.Problems) > 0 {
		label = "配置有误：" + ce.Problems[0].String()
		if n := len(ce.Problems); n > 1 {
			label += "（共 " + strconv.Itoa(n) + " 项）"
		}
	}
	item.SetLabel(label)
	item.SetVisible(true)
	sep.SetVisible(true)
	a.tray.SetToolTip("Gloss\n" + s.Err.Error())
}

func (a *app) logSnapshot(s config.Snapshot) {
	if s.Err != nil {
		a.log.Warn("config invalid, keeping the last valid one", "problems", s.Err.Error())
		return
	}
	a.log.Info("config loaded", "models", len(s.Config.Models), "modes", len(s.Config.Modes))
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
