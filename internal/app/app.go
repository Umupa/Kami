// Package app runs the menu bar app: its lifecycle, the tray, the watch on
// config.json, and the entry points the panel adds.
package app

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strconv"
	"time"

	"github.com/egoist/mygo"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/hotkey"
	"github.com/Umupa/cida-next/internal/keyring"
	"github.com/Umupa/cida-next/internal/llm"
	"github.com/Umupa/cida-next/internal/macos"
	"github.com/Umupa/cida-next/internal/mode"
	"github.com/Umupa/cida-next/internal/panel"
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
	// doubleTap watches for the panel's double tap; nil when the panel
	// shortcut is a key combination. Main thread only.
	doubleTap *hotkey.DoubleTap
	tapping   bool
}

// Run starts the app and returns when it quits. A second launch returns at
// once and leaves the running instance in charge.
func Run() error {
	if !mygo.App.RequestSingleInstanceLock() {
		return nil
	}
	mygo.App.SetActivationPolicy(mygo.ActivationPolicyAccessory)

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
	a.logSnapshot(a.store.Snapshot())

	ctx, stop := context.WithCancel(context.Background())
	mygo.App.OnSecondInstance(func([]string, string) {
		if a.panel != nil {
			a.panel.Show()
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
		Store:   a.store,
		Keyring: keyring.System(),
		Client:  func(config.Model) llm.Client { return llm.HTTP{} },
		Log:     a.log,
		Demo:    mygo.IsDev(),
	})
	a.hotkeys = hotkey.NewManager(a.log)

	a.menu = mygo.NewMenu([]*mygo.MenuItem{
		{ID: "config-error", Hidden: true, Click: a.revealConfig},
		{ID: "config-error-sep", Type: mygo.MenuItemSeparator, Hidden: true},
		{Label: "显示面板", Click: func(*mygo.MenuItem, *mygo.Window) { a.panel.Show() }},
		{ID: "listen-access", Label: "允许「输入监控」，以双击 ⌥ 打开面板…", Hidden: true, Click: a.requestListenAccess},
		mygo.Separator(),
		{Label: "复制配置提示词", Click: a.copyAgentPrompt},
		{Label: "打开配置文件夹", Click: a.revealConfig},
		mygo.Separator(),
		{Label: "退出 cida-next", Role: mygo.RoleQuit},
	})
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:           trayIcon(false),
		IconIsTemplate: true,
		ToolTip:        "cida-next",
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
	a.store.Subscribe(func(s config.Snapshot) {
		a.logSnapshot(s)
		a.showSnapshot(s)
		mygo.RunOnMain(func() { a.applyShortcuts(s.Config) })
	})
	go a.watchConfig(ctx)
}

// applyShortcuts registers the global shortcuts of c. Until selection import
// lands, the selection shortcut shows the panel as it does without a
// selection.
func (a *app) applyShortcuts(c *config.Config) {
	a.hotkeys.Apply([]hotkey.Binding{
		{Name: "panel", Shortcut: c.Shortcuts.Panel, Action: a.panel.Toggle},
		{Name: "selection", Shortcut: c.Shortcuts.Selection, Action: a.panel.Toggle},
	})
	a.doubleTap = nil
	if sc, err := config.ParseShortcut(c.Shortcuts.Panel); err == nil && sc.Double != 0 {
		a.doubleTap = &hotkey.DoubleTap{Mod: sc.Double, Window: hotkey.DoubleTapWindow}
		a.watchKeys()
	}
	a.menu.ItemByID("listen-access").SetVisible(a.doubleTap != nil && !a.tapping)
}

// watchKeys starts the event tap that sees double taps, once Input
// Monitoring is granted.
func (a *app) watchKeys() {
	if a.tapping {
		return
	}
	err := macos.WatchKeys(func(ev macos.KeyEvent) {
		if d := a.doubleTap; d != nil && d.Feed(time.Now(), hotkey.ModifiersOf(ev.Flags), ev.KeyDown) {
			a.panel.Toggle()
		}
	})
	if err != nil {
		a.log.Warn("watch keys", "err", err)
		return
	}
	a.tapping = true
}

// requestListenAccess asks for Input Monitoring. macOS applies a new grant
// to an event tap only after the app restarts, so the item says so.
func (a *app) requestListenAccess(item *mygo.MenuItem, _ *mygo.Window) {
	if !macos.RequestListenAccess() {
		_ = mygo.Shell.OpenExternal("x-apple.systempreferences:com.apple.preference.security?Privacy_ListenEvent")
	}
	a.watchKeys()
	if a.tapping {
		item.SetVisible(false)
		return
	}
	item.SetLabel("授权「输入监控」后，退出并重新打开 cida-next")
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
		a.tray.SetToolTip("cida-next")
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
	a.tray.SetToolTip("cida-next\n" + s.Err.Error())
}

func (a *app) logSnapshot(s config.Snapshot) {
	if s.Err != nil {
		a.log.Warn("config invalid, keeping the last valid one", "problems", s.Err.Error())
		return
	}
	a.log.Info("config loaded", "models", len(s.Config.Models), "modes", len(s.Config.Modes))
}

// copyAgentPrompt copies the prompt that has an AI assistant configure the
// app. Reading the keychain may wait on the user, so it runs off the main
// thread.
func (a *app) copyAgentPrompt(*mygo.MenuItem, *mygo.Window) {
	exe, err := mygo.App.Path(mygo.PathExe)
	if err != nil {
		a.log.Error("executable path", "err", err)
		return
	}
	snap := a.store.Snapshot()
	go func() {
		keys := keyring.System()
		prompt := config.AgentPrompt(config.PromptInput{
			Exe:      exe,
			Dir:      a.configDir,
			Snapshot: snap,
			HasKey: func(model string) bool {
				_, err := keys.Get(model)
				return err == nil
			},
		})
		mygo.Clipboard.WriteText(prompt)
	}()
}

func (a *app) revealConfig(*mygo.MenuItem, *mygo.Window) {
	mygo.Shell.ShowItemInFolder(a.store.Path())
}
