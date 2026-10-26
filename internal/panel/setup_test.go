package panel

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/llm"
	"github.com/Umupa/Kami/internal/operator"
)

// setupHarness drives a panel with the app's real configuration prompt.
type setupHarness struct {
	*harness
}

func newSetupHarness(t *testing.T, client llm.Client, configJSON string) *setupHarness {
	sh := &setupHarness{harness: newHarnessWith(t, client, configJSON)}
	sh.p.deps.AgentPrompt = func() string {
		return config.AgentPrompt(config.PromptInput{
			Exe:    "/Applications/Kami.app/Contents/MacOS/Kami",
			Dir:    "/Users/me/Library/Application Support/Kami",
			Config: sh.p.deps.Store.Snapshot().Config,
		})
	}
	return sh
}

// show greets as the panel does when it shows, and lets the reply stream.
func (sh *setupHarness) show() {
	sh.p.greet()
	sh.drain()
}

// arrive writes config.json and has the panel take it, as the watch does.
func (sh *setupHarness) arrive(configJSON string) {
	sh.t.Helper()
	if err := os.WriteFile(sh.p.deps.Store.Path(), []byte(configJSON), 0o600); err != nil {
		sh.t.Fatal(err)
	}
	sh.p.applyConfig(sh.p.deps.Store.Reload().Config)
	sh.drain()
}

const firstModel = `{"version":1,"model":{"endpoint":"https://api.deepseek.com/chat/completions","format":"chat-completions","model":"deepseek-chat","apiKey":"k"}}`

func TestFirstRunIsAConversation(t *testing.T) {
	sh := newSetupHarness(t, script{pieces: []string{"Bonjour"}}, `{"version":1}`)
	sh.tt.SetDark(true)
	sh.show()
	prompt := sh.p.agentPrompt()
	if !sh.p.greeting() || !strings.Contains(sh.p.sess.Last().Reply, strings.TrimSpace(prompt)) {
		t.Fatalf("the panel did not answer with the prompt; greeting %v", sh.p.greeting())
	}
	if !sh.tt.HasText("欢迎使用 Kami。") || sh.tt.HasText("执行") {
		t.Fatalf("texts %q: want the reply, without the run button", sh.tt.Texts())
	}
	if _, ok := sh.tt.Find("追问"); ok {
		t.Error("the app's own reply offers a follow-up")
	}
	sh.snapshot("first-run")

	sh.tt.Click("原文")
	sh.tt.Type("Hello world")
	sh.tt.Key(0, ui.KeyEnter)
	sh.tt.Frame()
	if sh.clip != prompt || sh.p.source != "Hello world" || !sh.p.greeting() {
		t.Fatalf("Enter copied %d bytes, source %q: want the prompt copied and the source kept", len(sh.clip), sh.p.source)
	}
	if _, ok := sh.tt.Find("已复制"); !ok {
		t.Error("the copy did not show")
	}

	// Another mode keeps the reply: it belongs to none.
	sh.tt.Key(0, ui.KeyTab)
	if !sh.p.greeting() {
		t.Error("switching modes dropped the reply")
	}
	sh.tt.Key(ui.Shift, ui.KeyTab)

	sh.arrive(firstModel)
	if !strings.Contains(sh.p.sess.Last().Reply, "正在使用 deepseek-chat") || !sh.tt.HasText("执行") {
		t.Fatalf("texts %q: want the panel to say the model is connected", sh.tt.Texts())
	}
	if reply := sh.p.sess.Last().Reply; !strings.Contains(reply, "用「翻译」处理上面的文字") || !strings.Contains(reply, "| `⌥Space` | 打开面板 |") {
		t.Errorf("connected reply %q: want what to try and how to call Kami up", reply)
	}
	sh.snapshot("first-run-connected")

	sh.tt.Click("原文")
	sh.tt.Key(0, ui.KeyEnter)
	sh.drain()
	if sh.p.greeting() || sh.p.sess.Last().Reply != "Bonjour" {
		t.Errorf("Enter after connecting: reply %q, want the model's", sh.p.sess.Last().Reply)
	}
}

func TestFirstRunCopyButton(t *testing.T) {
	sh := newSetupHarness(t, script{}, `{"version":1}`)
	sh.tt.Frame()
	sh.show()
	if err := sh.tt.Click("复制配置提示词 ⌘C"); err != nil {
		t.Fatalf("%v; texts %q", err, sh.tt.Texts())
	}
	if sh.clip != sh.p.agentPrompt() {
		t.Errorf("the copy button copied %q, want the prompt alone", sh.clip)
	}
}

func TestFirstRunBrokenConfig(t *testing.T) {
	sh := newSetupHarness(t, script{}, `{"version":1,"model":{"endpoint":"api.example.com","format":"chat","model":""}}`)
	sh.tt.SetDark(true)
	sh.show()
	reply := sh.p.sess.Last().Reply
	for _, want := range []string{"**config.json 有错误**", "- `model.endpoint`：必须是完整的 http(s) 地址", "运行 `Kami check`，按结果走"} {
		if !strings.Contains(reply, want) {
			t.Errorf("reply lacks %q:\n%s", want, reply)
		}
	}
	sh.snapshot("first-run-broken")
}

func TestFirstRunImportsWait(t *testing.T) {
	sh := newSetupHarness(t, script{}, `{"version":1}`)
	sh.tt.Frame()
	sh.p.importCaptured("Text in a screenshot")
	sh.drain()
	if sh.p.source != "Text in a screenshot" || !sh.p.greeting() || sh.clip != "" {
		t.Errorf("captured text with no model: source %q, greeting %v, copied %q; want it waiting in the source", sh.p.source, sh.p.greeting(), sh.clip)
	}
}

func TestFirstRunNewSessionGreetsAgain(t *testing.T) {
	sh := newSetupHarness(t, script{}, `{"version":1}`)
	sh.tt.Frame()
	sh.show()
	sh.p.reset()
	sh.drain()
	if !sh.p.greeting() {
		t.Error("⌘N with no model left the panel without the prompt")
	}
}

func TestRejectedRequestCopiesFixPrompt(t *testing.T) {
	rejected := &llm.Error{Kind: llm.KindConfig, Status: 401, Detail: `{"error":{"message":"Incorrect API key provided: ••••"}}`}
	sh := newSetupHarness(t, clientFunc(func(context.Context, llm.Request, func(string)) error { return rejected }), testConfig)
	sh.tt.SetDark(true)
	sh.tt.Frame()
	sh.tt.Click("原文")
	sh.tt.Type("Hello")
	sh.tt.Key(0, ui.KeyEnter)
	sh.drain()
	if !sh.tt.HasText("⌘C 复制提示词") {
		t.Fatalf("texts %q: want the way to hand the failure on", sh.tt.Texts())
	}
	sh.tt.Key(ui.Cmd, ui.KeyC)
	sh.tt.Frame()
	if sh.clip != sh.p.agentPrompt() || strings.Contains(sh.clip, testAPIKey) {
		t.Errorf("copied %q: want the configuration prompt, never the key", sh.clip)
	}
	if !sh.tt.HasText("已复制") {
		t.Errorf("texts %q: want the copy confirmed", sh.tt.Texts())
	}
	sh.snapshot("failure-config")
}

// permissionHarness is a connected panel whose permissions are fakes the
// test grants, and whose permission step is recorded as the app keeps it.
type permissionHarness struct {
	*setupHarness
	granted    map[string]bool
	requested  []string
	relaunched int
	pending    bool
}

func newPermissionHarness(t *testing.T) *permissionHarness {
	ph := &permissionHarness{setupHarness: newSetupHarness(t, script{}, `{"version":1}`), granted: map[string]bool{}}
	ph.p.deps.Permissions = func() []operator.Permission {
		var ps []operator.Permission
		for _, id := range []string{operator.PermAccessibility, operator.PermInputMonitoring, operator.PermScreenRecording} {
			// Granted Input Monitoring and Screen Recording wait for a
			// restart, as they do in a run that started without them.
			ps = append(ps, operator.Permission{ID: id, Name: operator.PermissionName(id), Granted: ph.granted[id],
				Restart: ph.granted[id] && id != operator.PermAccessibility})
		}
		return ps
	}
	ph.p.deps.RequestPermission = func(id string) { ph.requested = append(ph.requested, id) }
	ph.p.deps.Relaunch = func() { ph.relaunched++ }
	ph.p.deps.OnSetup = func(pending bool) { ph.pending = pending }
	ph.tt.Frame()
	ph.show()
	return ph
}

func (ph *permissionHarness) enter() {
	ph.tt.Click("原文")
	ph.tt.Key(0, ui.KeyEnter)
	ph.tt.Frame()
}

func TestFirstRunHoldsThePanel(t *testing.T) {
	ph := newPermissionHarness(t)
	if !ph.p.heldOpen || ph.p.pinned {
		t.Fatalf("held %v, pinned %v: want the panel to stay up, below System Settings and prompts", ph.p.heldOpen, ph.p.pinned)
	}
	ph.arrive(firstModel)
	if !ph.p.heldOpen || !ph.pending {
		t.Fatalf("held %v, step pending %v: want the panel held for the permission step", ph.p.heldOpen, ph.pending)
	}
	// Using Kami ends the step, and lets the panel go; what is left is
	// asked for on use.
	ph.tt.Click("原文")
	ph.tt.Type("Hello")
	if !strings.Contains(ph.p.sess.Last().Reply, "按 ⏎，用「翻译」处理上面的文字。") {
		t.Errorf("with text to process, reply:\n%s", ph.p.sess.Last().Reply)
	}
	ph.enter()
	if ph.pending || ph.p.heldOpen || len(ph.requested) > 0 {
		t.Errorf("pending %v, held %v after the user went on: want the step over", ph.pending, ph.p.heldOpen)
	}
}

// The panel floats but while held for the permission step; unpinning must
// not leave it at the normal level, where the window of the app the user
// just switched to covers it as it shows.
func TestPanelLevel(t *testing.T) {
	ph := newPermissionHarness(t)
	if ph.p.onTop() {
		t.Error("held for the permission step, the panel floats over System Settings")
	}
	ph.p.togglePin()
	if !ph.p.onTop() {
		t.Error("pinned while held, the panel does not float")
	}
	ph.p.togglePin()
	if ph.p.onTop() {
		t.Error("unpinned while held, the panel still floats")
	}
	ph.p.endSetup()
	if !ph.p.onTop() {
		t.Error("after the permission step, the panel does not float")
	}
	ph.p.togglePin()
	ph.p.togglePin()
	if !ph.p.onTop() {
		t.Error("pinned and unpinned, the panel dropped to the normal level")
	}
}

func TestConnectedAsksForPermissionsOneAtATime(t *testing.T) {
	ph := newPermissionHarness(t)
	ph.arrive(firstModel)
	reply := ph.p.sess.Last().Reply
	for _, want := range []string{
		"接下来授予以下权限，以使用完整功能：\n\n- [ ] 辅助功能　双击 ⌥ 带入选中的文字\n- [ ] 输入监控　识别双击 ⌥\n- [ ] 屏幕录制　⌥S 截图识别\n",
		"点「授权」或按 ⏎，在打开的系统设置里打开 Kami 的「辅助功能」开关。",
	} {
		if !strings.Contains(reply, want) {
			t.Fatalf("connected reply lacks %q:\n%s", want, reply)
		}
	}
	if !ph.tt.HasText("授权") || ph.tt.HasText("执行") {
		t.Errorf("texts %q: want the run button to say what Enter does", ph.tt.Texts())
	}
	ph.snapshot("first-run-permissions")

	ph.enter()
	if strings.Join(ph.requested, ",") != operator.PermAccessibility {
		t.Fatalf("requested %q: want Accessibility first, which needs no restart", ph.requested)
	}
	if !ph.tt.HasText("授权") {
		t.Errorf("texts %q: asking once is enough, the button stays", ph.tt.Texts())
	}

	// Grants made in System Settings show as the panel polls.
	ph.granted[operator.PermAccessibility] = true
	ph.tt.Frame()
	if reply := ph.p.sess.Last().Reply; !strings.Contains(reply, "- [x] 辅助功能") || !strings.Contains(reply, "打开 Kami 的「输入监控」开关") {
		t.Fatalf("after a grant, reply:\n%s", reply)
	}
	ph.enter()
	ph.granted[operator.PermInputMonitoring] = true
	ph.enter()
	ph.granted[operator.PermScreenRecording] = true
	ph.tt.Frame()
	if want := []string{operator.PermAccessibility, operator.PermInputMonitoring, operator.PermScreenRecording}; strings.Join(ph.requested, ",") != strings.Join(want, ",") {
		t.Fatalf("requested %q, want %q", ph.requested, want)
	}
	if reply := ph.p.sess.Last().Reply; !strings.Contains(reply, "点「重新启动」或按 ⏎，让「输入监控」和「屏幕录制」生效。") || !ph.pending {
		t.Fatalf("all granted, pending %v, reply:\n%s", ph.pending, reply)
	}
	ph.enter()
	if ph.relaunched != 1 || !ph.pending {
		t.Errorf("relaunched %d times, pending %v: want one restart, the step resuming after it", ph.relaunched, ph.pending)
	}
}

// A grant often restarts Kami before the step is through; the next launch
// picks it up where it was.
func TestPermissionStepResumesAfterRestart(t *testing.T) {
	ph := newPermissionHarness(t)
	ph.arrive(firstModel)
	ph.granted[operator.PermAccessibility] = true
	ph.granted[operator.PermInputMonitoring] = true

	next := newPermissionHarness(t)
	next.granted = ph.granted
	next.arrive(firstModel) // the model of the last run, already there
	next.p.reset()
	next.drain()
	if !next.p.ResumeSetup() {
		t.Fatal("there was no step to resume")
	}
	next.drain()
	reply := next.p.sess.Last().Reply
	if !next.p.welcoming() || !next.p.heldOpen || !strings.Contains(reply, "- [x] 输入监控") || !strings.Contains(reply, "- [ ] 屏幕录制") {
		t.Fatalf("welcoming %v, held %v, reply:\n%s", next.p.welcoming(), next.p.heldOpen, reply)
	}

	// With every permission in effect there is nothing to resume.
	done := newPermissionHarness(t)
	done.arrive(firstModel)
	done.p.deps.Permissions = func() []operator.Permission {
		return []operator.Permission{{ID: operator.PermAccessibility, Granted: true}, {ID: operator.PermInputMonitoring, Granted: true}, {ID: operator.PermScreenRecording, Granted: true}}
	}
	if !done.p.ResumeSetup() || done.pending || done.p.heldOpen {
		t.Fatal("a step with nothing left did not end")
	}
	done.drain()
	reply = done.p.sess.Last().Reply
	for _, want := range []string{"🎉 **Kami 已准备就绪**，正在使用 deepseek-chat（api.deepseek.com）。常用操作：", "| `⌥Space` | 打开面板 |", "| 双击 `⌥` | 带入选中的文字，立即翻译 |", "| `⌥S` | 截图识别文字，立即翻译 |", "| `Tab` | 切换模式：翻译、润色、问答、笔记 |", "输入或粘贴一段文字，按 ⏎ 试一次。"} {
		if !strings.Contains(reply, want) {
			t.Errorf("ready reply lacks %q:\n%s", want, reply)
		}
	}
	done.snapshot("first-run-ready")
}
