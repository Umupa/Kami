package panel

import (
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// updateHarness is a panel with a newer version, whose choices it records.
type updateHarness struct {
	*harness
	state            UpdateState
	offered          bool
	calls            []string
	installed, later bool
}

func newUpdateHarness(t *testing.T) *updateHarness {
	uh := &updateHarness{harness: newHarnessWith(t, script{pieces: []string{"Bonjour"}}, testConfig), offered: true,
		state: UpdateState{Phase: UpdateAvailable, Current: "0.1.0", Version: "0.2.0",
			Notes: "- 菜单栏显示模型与连接状态\n- 授权一项一项来"}}
	uh.p.deps.Updates = Updates{
		Offer: func() (UpdateState, bool) { return uh.state, uh.offered && !uh.later },
		State: func() (UpdateState, bool) { return uh.state, true },
		Install: func() {
			uh.calls = append(uh.calls, "install")
			uh.state.Phase, uh.state.Progress = UpdateDownloading, -1
		},
		Later:   func() { uh.calls = append(uh.calls, "later"); uh.later = true },
		Skip:    func() { uh.calls = append(uh.calls, "skip") },
		Restart: func() { uh.calls = append(uh.calls, "restart") },
	}
	uh.tt.Frame()
	return uh
}

func TestUpdateOfferedAsThePanelOpens(t *testing.T) {
	uh := newUpdateHarness(t)
	uh.p.offerUpdate()
	uh.tt.Frame()
	for _, want := range []string{"Kami 0.2.0 可以安装", " · 当前 0.1.0", "安装并重启", "稍后", "跳过这个版本", "更新内容"} {
		if !uh.tt.HasText(want) {
			t.Fatalf("texts %q lack %q", uh.tt.Texts(), want)
		}
	}
	if uh.tt.HasText("执行") {
		t.Error("the update screen keeps the run button")
	}
	uh.snapshot("update")

	// Tab moves along the bar; Enter takes the choice.
	uh.tt.Key(0, ui.KeyTab)
	uh.tt.Key(0, ui.KeyEnter)
	uh.tt.Frame()
	if strings.Join(uh.calls, ",") != "later" || uh.p.updating {
		t.Fatalf("calls %q, updating %v: want it put off and the session back", uh.calls, uh.p.updating)
	}
	if !uh.tt.HasText("执行") {
		t.Errorf("texts %q: want the panel back", uh.tt.Texts())
	}
	// Put off, it is not offered again in this run.
	uh.p.offerUpdate()
	if uh.p.updating {
		t.Error("an update put off was offered again")
	}
}

func TestUpdateInstallsAndRestartsWhileThePanelIsUp(t *testing.T) {
	uh := newUpdateHarness(t)
	uh.p.openUpdate()
	uh.tt.Frame()
	uh.tt.Key(0, ui.KeyEnter)
	uh.tt.Frame()
	if strings.Join(uh.calls, ",") != "install" || !uh.tt.HasText("下载中…") {
		t.Fatalf("calls %q, texts %q: want the download under way", uh.calls, uh.tt.Texts())
	}
	uh.state.Progress = 0.42
	uh.tt.Advance(200e6)
	if !uh.tt.HasText("下载中 42%") {
		t.Fatalf("texts %q: want the progress", uh.tt.Texts())
	}
	uh.state.Phase = UpdateReady
	uh.tt.Advance(200e6)
	if strings.Join(uh.calls, ",") != "install,restart" {
		t.Errorf("calls %q: want the restart the user asked for", uh.calls)
	}
}

func TestUpdateReadyAfterLeavingWaitsForTheUser(t *testing.T) {
	uh := newUpdateHarness(t)
	uh.state.Phase = UpdateReady
	uh.p.updateConfirmed = false // the user left while it downloaded
	uh.p.openUpdate()
	uh.tt.Frame()
	if !uh.tt.HasText("Kami 0.2.0 已安装") || !uh.tt.HasText("立即重启") || len(uh.calls) > 0 {
		t.Fatalf("texts %q, calls %q: want the restart offered, not done", uh.tt.Texts(), uh.calls)
	}
	uh.tt.Key(0, ui.KeyEnter)
	uh.tt.Frame()
	uh.tt.Frame()
	if strings.Join(uh.calls, ",") != "restart" {
		t.Errorf("calls %q: want the restart", uh.calls)
	}
}

func TestUpdateSkipAndEscape(t *testing.T) {
	uh := newUpdateHarness(t)
	uh.p.openUpdate()
	uh.tt.Frame()
	uh.tt.Key(ui.Shift, ui.KeyTab) // around to the last
	uh.tt.Key(0, ui.KeyEnter)
	uh.tt.Frame()
	if strings.Join(uh.calls, ",") != "skip" || uh.p.updating {
		t.Fatalf("calls %q: want the version skipped", uh.calls)
	}
	uh.p.openUpdate()
	uh.tt.Frame()
	uh.tt.Key(0, ui.KeyEscape)
	uh.tt.Frame()
	if strings.Join(uh.calls, ",") != "skip,later" || uh.p.updating {
		t.Errorf("calls %q: want Escape to put it off", uh.calls)
	}
}

func TestUpdateNotOfferedDuringSetup(t *testing.T) {
	uh := newUpdateHarness(t)
	uh.p.inSetup = true
	uh.p.offerUpdate()
	if uh.p.updating {
		t.Error("the update screen took over the permission step")
	}
}

// checkHarness is a panel whose /update check the test answers.
func newCheckHarness(t *testing.T) *updateHarness {
	uh := newUpdateHarness(t)
	uh.state = UpdateState{}
	uh.p.deps.Updates.State = func() (UpdateState, bool) { return uh.state, uh.state.Phase != 0 }
	uh.p.deps.Updates.Later = func() { uh.calls = append(uh.calls, "later"); uh.state = UpdateState{} }
	uh.p.deps.Updates.Check = func() {
		uh.calls = append(uh.calls, "check")
		uh.state = UpdateState{Phase: UpdateChecking, Current: "0.1.0"}
	}
	uh.p.deps.Updates.Version = "0.1.0"
	return uh
}

func TestUpdateCommandChecksAndShowsTheVersion(t *testing.T) {
	uh := newCheckHarness(t)
	uh.tt.Click("原文")
	uh.tt.Type("/upd")
	waitForOperatorMenu(t, uh.harness, "/update", "检查更新", "当前 0.1.0")
	uh.snapshotNow("update-command")
	uh.tt.Key(0, ui.KeyEnter)
	uh.tt.Frame()
	if strings.Join(uh.calls, ",") != "check" || !uh.p.updating || uh.p.source != "" {
		t.Fatalf("calls %q, updating %v, source %q: want a check on its screen", uh.calls, uh.p.updating, uh.p.source)
	}
	for _, want := range []string{"正在检查更新", " · 当前 0.1.0", "检查中…", "关闭"} {
		if !uh.tt.HasText(want) {
			t.Fatalf("texts %q lack %q", uh.tt.Texts(), want)
		}
	}
	if uh.tt.HasText("更新内容") {
		t.Error("a check shows release notes")
	}

	uh.state.Phase = UpdateLatest
	uh.tt.Advance(200 * time.Millisecond)
	if !uh.tt.HasText("Kami 0.1.0 已是最新版本") || !uh.tt.HasText("好") {
		t.Fatalf("texts %q: want the latest version confirmed", uh.tt.Texts())
	}
	if uh.tt.HasText("切换") {
		t.Error("a single choice offers Tab")
	}
	uh.snapshot("update-latest")
	uh.tt.Key(0, ui.KeyEscape)
	uh.tt.Frame()
	if strings.Join(uh.calls, ",") != "check,later" || uh.p.updating || !uh.tt.HasText("执行") {
		t.Errorf("calls %q, updating %v: want Escape to close the check", uh.calls, uh.p.updating)
	}
}

func TestUpdateCheckFailureRetries(t *testing.T) {
	uh := newCheckHarness(t)
	uh.state = UpdateState{Phase: UpdateCheckFailed, Current: "0.1.0"}
	uh.p.ShowUpdate()
	uh.tt.Frame()
	if !uh.tt.HasText("检查更新失败") || !uh.tt.HasText("重试") {
		t.Fatalf("texts %q: want the failure and a retry", uh.tt.Texts())
	}
	uh.tt.Key(0, ui.KeyEnter)
	uh.tt.Frame()
	if strings.Join(uh.calls, ",") != "check" || !uh.p.updating || !uh.tt.HasText("正在检查更新") {
		t.Errorf("calls %q, texts %q: want the check again", uh.calls, uh.tt.Texts())
	}
	// A newer version found by the check takes the screen over.
	uh.state = UpdateState{Phase: UpdateAvailable, Current: "0.1.0", Version: "0.2.0", Notes: "- 新功能"}
	uh.tt.Advance(200 * time.Millisecond)
	if !uh.tt.HasText("Kami 0.2.0 可以安装") || !uh.tt.HasText("安装并重启") {
		t.Errorf("texts %q: want the newer version offered", uh.tt.Texts())
	}
}

func TestUpdateCommandNeedsAChecker(t *testing.T) {
	h := newHarnessWith(t, script{pieces: []string{"Bonjour"}}, testConfig)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("/upd")
	for range operatorMenuFrames {
		h.tt.Frame()
	}
	if h.tt.HasText("检查更新") {
		t.Error("/update shows without a way to check")
	}
}

// The panel grows to show the notes: the window is as tall as the panel was
// before, and the notes' scroll area alone would keep to it.
func TestUpdateNotesGrowThePanel(t *testing.T) {
	for _, notes := range []string{
		"- 菜单栏显示模型与连接状态\n- 授权一项一项来",
		"- 修复取消固定后，在副屏等处用快捷键唤起的面板被其他应用的窗口挡住、看起来没有弹出的问题。",
	} {
		uh := newUpdateHarness(t)
		uh.state.Notes = notes
		uh.settle()
		before := uh.p.height
		uh.tt.SetSize(820, before)
		uh.tt.Frame()
		uh.p.offerUpdate()
		for range 4 {
			uh.tt.SetSize(820, uh.p.height)
			uh.settle()
		}
		lines := strings.Split(notes, "\n")
		last, ok := uh.tt.Find(strings.TrimPrefix(lines[len(lines)-1], "- "))
		if !ok || last.H == 0 || last.Y+last.H > float32(uh.p.height) {
			t.Fatalf("panel %d tall (was %d), last note at %v: want the notes in view", uh.p.height, before, last)
		}
		uh.tt.Key(0, ui.KeyEscape)
		for range 3 {
			uh.tt.SetSize(820, uh.p.height)
			uh.settle()
		}
		if uh.p.updating || uh.p.height != before {
			t.Errorf("updating %v, height %d after closing: want the panel back at %d", uh.p.updating, uh.p.height, before)
		}
	}
}

// A height the user resized the panel to gives way to the notes while they
// show, and comes back as the screen closes.
func TestUpdateNotesGrowAResizedPanel(t *testing.T) {
	uh := newUpdateHarness(t)
	uh.state.Notes = "- 修复取消固定后，在副屏等处用快捷键唤起的面板被其他应用的窗口挡住、看起来没有弹出的问题。"
	uh.settle()
	short := uh.p.height + 3
	uh.p.resized(mygo.Rectangle{Width: 820, Height: short})
	uh.tt.SetSize(820, short)
	uh.settle()
	uh.p.offerUpdate()
	for range 4 {
		uh.tt.SetSize(820, uh.p.height)
		uh.settle()
	}
	last, ok := uh.tt.Find("修复取消固定后，在副屏等处用快捷键唤起的面板被其他应用的窗口挡住、看起来没有弹出的问题。")
	if !ok || last.H == 0 || last.Y+last.H > float32(uh.p.height) {
		t.Fatalf("panel %d tall (resized to %d), last note at %v: want the notes in view", uh.p.height, short, last)
	}
	uh.tt.Key(0, ui.KeyEscape)
	for range 3 {
		uh.tt.SetSize(820, uh.p.height)
		uh.settle()
	}
	if uh.p.fixedHeight != short || uh.p.height != short {
		t.Errorf("fixed %d, height %d after closing: want the user's %d back", uh.p.fixedHeight, uh.p.height, short)
	}
}
