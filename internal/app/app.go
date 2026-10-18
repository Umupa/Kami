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
)

// app is the running application. Menu and tray methods are safe from any
// goroutine; everything else is set up before the event loop starts.
type app struct {
	log       *slog.Logger
	configDir string
	store     *config.Store
	tray      *mygo.Tray
	menu      *mygo.Menu
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
	a.store = config.NewStore(filepath.Join(dataDir, config.FileName), nil)
	a.logSnapshot(a.store.Snapshot())

	ctx, stop := context.WithCancel(context.Background())
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
	a.menu = mygo.NewMenu([]*mygo.MenuItem{
		{ID: "config-error", Hidden: true, Click: a.revealConfig},
		{ID: "config-error-sep", Type: mygo.MenuItemSeparator, Hidden: true},
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
	a.store.Subscribe(func(s config.Snapshot) {
		a.logSnapshot(s)
		a.showSnapshot(s)
	})
	go a.watchConfig(ctx)
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

func (a *app) revealConfig(*mygo.MenuItem, *mygo.Window) {
	mygo.Shell.ShowItemInFolder(a.store.Path())
}
