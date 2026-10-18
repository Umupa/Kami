package app

import (
	"os"
	"strings"

	"github.com/egoist/mygo"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/macos"
	"github.com/Umupa/cida-next/internal/mode"
	"github.com/Umupa/cida-next/internal/operator"
)

// permissions are the system permissions the app uses, with the System
// Settings pages that grant them.
var permissions = []struct {
	id, name, purpose, page string
	granted                 func() bool
	request                 func() bool
}{
	{"accessibility", "辅助功能", "带入选中的文字", "Privacy_Accessibility", macos.AXTrusted, macos.RequestAXTrust},
	{"input-monitoring", "输入监控", "双击修饰键打开面板", "Privacy_ListenEvent", macos.ListenAccess, macos.RequestListenAccess},
	{"screen-recording", "屏幕录制", "截图识别", "Privacy_ScreenCapture", macos.ScreenAccess, macos.RequestScreenAccess},
}

// operatorState describes the app to the operator.
func (a *app) operatorState() operator.State {
	file, _ := os.ReadFile(a.store.Path())
	s := operator.State{
		Version:     mygo.App.Version(),
		ConfigPath:  a.store.Path(),
		File:        file,
		Config:      a.store.Snapshot().Config,
		AgentPrompt: a.agentPrompt(),
	}
	for _, p := range permissions {
		s.Permissions = append(s.Permissions, operator.Permission{ID: p.id, Name: p.name, Purpose: p.purpose, Granted: p.granted()})
	}
	return s
}

// agentPrompt is the prompt that has a coding agent configure model
// services.
func (a *app) agentPrompt() string {
	exe, err := mygo.App.Path(mygo.PathExe)
	if err != nil {
		exe = "cida-next"
	}
	return config.AgentPrompt(config.PromptInput{Exe: exe, Dir: a.configDir, Snapshot: a.store.Snapshot()})
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
		id := strings.TrimPrefix(action, operator.ActionOpenPermission)
		for _, p := range permissions {
			if p.id == id && !p.request() {
				_ = mygo.Shell.OpenExternal("x-apple.systempreferences:com.apple.preference.security?" + p.page)
			}
		}
	default:
		a.log.Warn("operator action", "unknown", action)
	}
}
