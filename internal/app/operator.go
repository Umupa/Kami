package app

import (
	"os"
	"strings"
	"time"

	"github.com/egoist/mygo"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/design"
	"github.com/Umupa/Kami/internal/macos"
	"github.com/Umupa/Kami/internal/mode"
	"github.com/Umupa/Kami/internal/operator"
)

// permissions are the system permissions the app uses, with the System
// Settings pages that grant them.
var permissions = []struct {
	id, purpose, page string
	granted           func() bool
	request           func() bool
}{
	{operator.PermAccessibility, "带入选中的文字", "Privacy_Accessibility", macos.AXTrusted, macos.RequestAXTrust},
	{operator.PermInputMonitoring, "双击修饰键打开面板", "Privacy_ListenEvent", macos.ListenAccess, macos.RequestListenAccess},
	{operator.PermScreenRecording, "截图识别", "Privacy_ScreenCapture", macos.ScreenAccess, macos.RequestScreenAccess},
}

// operatorState describes the app to the operator.
func (a *app) operatorState() operator.State {
	file, _ := os.ReadFile(a.store.Path())
	s := operator.State{
		Version:     mygo.App.Version(),
		ConfigPath:  a.store.Path(),
		File:        file,
		Config:      a.store.Snapshot().Config,
		Permissions: a.permissionStates(),
	}
	s.Dark = mygo.Theme.IsDark()
	s.Palettes = make(map[string]map[string]string)
	for _, name := range design.ThemeNames {
		s.Palettes[name] = design.Themes[name].Colors()
	}
	if s.Config != nil {
		for _, t := range s.Config.Themes {
			s.Palettes[t.ID] = design.Derive(design.Themes[t.Base], t.Colors).Colors()
		}
	}
	return s
}

// permissionStates reads the permissions, and which of them wait for a
// restart. Main thread only: it may start watching keys.
func (a *app) permissionStates() []operator.Permission {
	var states []operator.Permission
	for _, p := range permissions {
		s := operator.Permission{ID: p.id, Name: operator.PermissionName(p.id), Purpose: p.purpose, Granted: p.granted()}
		switch {
		case !s.Granted:
		case p.id == operator.PermInputMonitoring && len(a.doubleTaps) > 0 && !a.tapping:
			// macOS may let the tap in without a restart; if not, it
			// takes one.
			a.watchKeys()
			s.Restart = !a.tapping
		case p.id == operator.PermScreenRecording:
			s.Restart = !a.screenAtLaunch
		}
		states = append(states, s)
	}
	return states
}

// requestPermission asks for a permission the user means to grant now: the
// macOS prompt, which also lists Kami on the permission's page of System
// Settings, then that page. The page opens a moment after the prompt: opened
// together, it shows the list before Kami is in it. Main thread only.
func (a *app) requestPermission(id string) {
	for _, p := range permissions {
		if p.id != id || p.granted() {
			continue
		}
		a.promptPermission(id)
		time.AfterFunc(settingsDelay, func() {
			mygo.RunOnMain(func() {
				if !p.granted() {
					a.log.Info("permission settings opened", "permission", id)
					_ = mygo.Shell.OpenExternal("x-apple.systempreferences:com.apple.preference.security?" + p.page)
				}
			})
		})
	}
}

// settingsDelay is how long System Settings waits after a permission prompt.
const settingsDelay = 1500 * time.Millisecond

// promptPermission shows macOS's prompt for a permission, which it does once
// per run at most. Main thread only.
func (a *app) promptPermission(id string) {
	for _, p := range permissions {
		if p.id != id || a.asked[id] {
			continue
		}
		a.log.Info("permission requested", "permission", id)
		if a.asked == nil {
			a.asked = map[string]bool{}
		}
		a.asked[id] = true
		p.request()
	}
}

// agentPrompt is the prompt that has a coding agent set up, fix or adjust
// the model.
func (a *app) agentPrompt() string {
	exe, err := mygo.App.Path(mygo.PathExe)
	if err != nil {
		exe = "Kami"
	}
	return config.AgentPrompt(config.PromptInput{Exe: exe, Dir: a.configDir, Config: a.store.Snapshot().Config})
}

// applyOps writes an operator edit to config.json, keeping the file before
// it for undo; the watch reloads the store as for any edit.
func (a *app) applyOps(ops []config.Op) ([]config.Change, error) {
	before, err := os.ReadFile(a.store.Path())
	if err != nil {
		return nil, err
	}
	after, changes, err := config.ApplyOps(before, ops, mode.Check)
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		return nil, nil
	}
	if err := config.WriteFile(a.store.Path(), after); err != nil {
		return nil, err
	}
	a.undo = before
	a.store.Reload()
	a.log.Info("operator changed config", "fields", len(changes))
	return changes, nil
}

// undoOps puts back config.json as it was before the last operator edit.
func (a *app) undoOps() bool {
	if a.undo == nil {
		return false
	}
	if err := config.WriteFile(a.store.Path(), a.undo); err != nil {
		a.log.Warn("operator undo", "err", err)
		return false
	}
	a.undo = nil
	a.store.Reload()
	return true
}

// runAction runs an action an operator reply asked for.
func (a *app) runAction(action string) {
	switch {
	case action == operator.ActionCopyAgentPrompt:
		mygo.Clipboard.WriteText(a.agentPrompt())
	case strings.HasPrefix(action, operator.ActionOpenPermission):
		a.requestPermission(strings.TrimPrefix(action, operator.ActionOpenPermission))
	default:
		a.log.Warn("operator action", "unknown", action)
	}
}
