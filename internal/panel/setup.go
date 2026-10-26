package panel

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/llm"
	"github.com/Umupa/Kami/internal/mode"
	"github.com/Umupa/Kami/internal/operator"
	"github.com/Umupa/Kami/internal/session"
)

// The first run is a conversation like any other (requirements F13): while
// there is no model, the panel answers with the prompt that has a coding
// agent configure one, and once one arrives, it says so. These replies are
// the app's own, streamed in as a model's are, and give way to the user's
// first request.

// kamiMode is the mode of the app's own replies.
var kamiMode = mode.Mode{ID: "kami", Name: "Kami", Output: config.OutputMarkdown}

// unconfigured reports whether there is no model to send anything to. A
// development build answering with canned replies is configured enough.
func (p *Panel) unconfigured() bool {
	return p.cfg != nil && p.cfg.Model == nil && !p.deps.Demo
}

// greeting reports whether the session shown is the app's own reply about
// its model.
func (p *Panel) greeting() bool {
	return p.sess != nil && !p.sess.Operator && p.sess.Turns[0].Local
}

// agentPrompt is the prompt for a coding agent, the same for a first run
// and for a rejected request: the agent finds out which with `Kami check`.
func (p *Panel) agentPrompt() string {
	if p.deps.AgentPrompt == nil {
		return ""
	}
	return p.deps.AgentPrompt()
}

// greet answers with the configuration prompt while there is no model,
// unless the user's own session shows. The same words are not said twice.
func (p *Panel) greet() {
	if !p.unconfigured() || p.noteMode() || p.sess != nil && !p.greeting() {
		return
	}
	// The user goes off to a coding agent with the prompt; the panel stays.
	p.holdForSetup()
	if text := p.setupText(); !p.greeting() || text != p.said {
		p.say(text)
	}
}

// connected says the first model arrived, in place of the configuration
// prompt or of nothing, and starts the permission step.
func (p *Panel) connected() {
	if p.sess != nil && !p.greeting() {
		// The user is busy with their own session; the step would only
		// keep the panel pinned.
		p.endSetup()
		return
	}
	p.say(p.connectedText())
	p.startSetup()
}

// ResumeSetup takes up the permission step again after a restart, which
// granting Input Monitoring or Screen Recording often takes: the permissions
// left, or, with all of them in effect, how to use Kami. It reports whether
// there is a reply to show.
func (p *Panel) ResumeSetup() bool {
	if p.unconfigured() || p.cfg == nil || p.cfg.Model == nil {
		return false
	}
	p.say(p.connectedText())
	p.startSetup()
	return true
}

// startSetup holds the panel for the permission step, which lasts across
// restarts until it is done.
func (p *Panel) startSetup() {
	p.inSetup = true
	if p.deps.OnSetup != nil {
		p.deps.OnSetup(true)
	}
	p.holdForSetup()
	if setupDone(p.permissionSteps()) {
		p.endSetup()
	}
}

// endSetup ends the permission step: every permission is in effect, or the
// user went on to use Kami.
func (p *Panel) endSetup() {
	p.inSetup = false
	if p.deps.OnSetup != nil {
		p.deps.OnSetup(false)
	}
	p.heldOpen = false
	p.applyLevel()
}

// holdForSetup keeps the panel up while the user works in a coding agent or
// System Settings. Unlike a pin it drops to the normal level, so System
// Settings and macOS's prompts come in front of it.
func (p *Panel) holdForSetup() {
	p.heldOpen = true
	p.applyLevel()
}

// setupDone reports whether every permission is granted and in effect.
func setupDone(steps []permStep) bool {
	return nextPermission(steps) == nil && restartNames(steps) == ""
}

// say answers with text the app wrote, streaming it in as a reply.
func (p *Panel) say(text string) {
	p.stop()
	p.said = text
	sess := session.New(kamiMode, "")
	t := sess.Last()
	t.Local, t.Status = true, session.Streaming
	p.sess, p.list = sess, newList()
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go func() {
		defer cancel()
		err := llm.Canned{Text: text}.Stream(ctx, llm.Request{}, func(s string) {
			p.post(func() { sess.Turns[0].Reply += s })
		})
		p.post(func() { p.finish(sess, 0, err) })
	}()
}

// setupText is what the panel says while there is no model: why nothing
// runs yet, and the prompt to hand on.
func (p *Panel) setupText() string {
	var b strings.Builder
	if err := p.configErr(); err != nil {
		b.WriteString("⚠️ **config.json 有错误**\n\n")
		for _, pr := range problemsOf(err) {
			if pr.Path != "" {
				fmt.Fprintf(&b, "- `%s`：%s\n", pr.Path, pr.Message)
			} else {
				fmt.Fprintf(&b, "- %s\n", pr.Message)
			}
		}
		b.WriteString("\n🛠️ 按 ⏎ 复制提示词，交给 Claude Code、Codex 等 Coding Agent 修复。\n\n")
	} else {
		b.WriteString("👋 **欢迎使用 Kami。**\n\n")
		b.WriteString("✨ 这是常驻在 Mac 上的 AI 文字助手，随时通过快捷键帮你翻译外文、润色表达与即时问答。\n\n")
		b.WriteString("🔌 Kami 由你自己的模型提供动力（支持 Claude、OpenAI、DeepSeek、本地 Ollama 等）。\n\n")
		b.WriteString("📋 按 **⏎** 复制下面的配置提示词给 **Claude Code**、**Codex** 等 Coding Agent，它会协助你完成接入并自动检查。配置完成后这里会接着显示，随时可以开始。\n\n")
	}
	b.WriteString("````text\n" + strings.TrimRight(p.agentPrompt(), "\n") + "\n````\n")
	return b.String()
}

// connectedText is what the panel says as the first model arrives: the
// permissions its entry points need, granted one at a time, then, with all
// of them in effect, how to use Kami.
func (p *Panel) connectedText() string {
	var b strings.Builder
	m := p.cfg.Model
	steps := p.permissionSteps()
	ready := setupDone(steps)
	if ready {
		fmt.Fprintf(&b, "🎉 **Kami 已准备就绪**，正在使用 %s（%s）。常用操作：\n\n", m.Model, hostOf(m.Endpoint))
		p.writeTips(&b)
	} else {
		fmt.Fprintf(&b, "✅ **已连接 %s**（%s）。", m.Model, hostOf(m.Endpoint))
		if sc, err := config.ParseShortcut(p.cfg.Shortcuts.Panel); err == nil && !sc.IsZero() {
			fmt.Fprintf(&b, "按 `%s` 随时打开 Kami。", sc.Glyphs())
		}
		b.WriteString("\n\n")
		if nextPermission(steps) != nil {
			b.WriteString("🔐 接下来授予以下权限，以使用完整功能：\n\n")
		}
		for _, s := range steps {
			mark := " "
			if s.Granted {
				mark = "x"
			}
			fmt.Fprintf(&b, "- [%s] %s　%s\n", mark, s.Name, s.use)
		}
		b.WriteString("\n")
	}
	next, restart := nextPermission(steps), restartNames(steps)
	switch {
	case strings.TrimSpace(p.source) != "" && len(p.modes) > 0 && !p.noteMode():
		// Text to process comes first; what is left is asked for on use.
		fmt.Fprintf(&b, "👉 按 ⏎，用「%s」处理上面的文字。\n", p.modes[p.modeIdx].Name)
	case next != nil:
		fmt.Fprintf(&b, "👉 点「授权」或按 ⏎，在打开的系统设置里打开 Kami 的「%s」开关。\n", next.Name)
	case restart != "":
		fmt.Fprintf(&b, "🔄 点「重新启动」或按 ⏎，让%s生效。\n", restart)
	default:
		b.WriteString("🚀 输入或粘贴一段文字，按 ⏎ 试一次。\n")
	}
	return b.String()
}

// writeTips lists the shortcuts of the entry points and of the panel.
func (p *Panel) writeTips(b *strings.Builder) {
	b.WriteString("| 按键 | 作用 |\n| --- | --- |\n")
	for _, e := range operator.Entries(p.cfg) {
		key := "`" + e.Shortcut.Glyphs() + "`"
		if e.Shortcut.Double != 0 {
			key = "双击 `" + strings.Join(e.Shortcut.Keys(), "") + "`"
		}
		fmt.Fprintf(b, "| %s | %s |\n", key, e.Does)
	}
	var names []string
	for _, m := range p.modes {
		names = append(names, m.Name)
	}
	fmt.Fprintf(b, "| `Tab` | 切换模式：%s、笔记 |\n", strings.Join(names, "、"))
	b.WriteString("| `⏎` | 执行；结果下方可以接着追问 |\n")
	b.WriteString("| `/` | 查看命令；`/operator` 用一句话修改设置 |\n\n")
}

// welcoming reports whether the session shown is the reply that says the
// model is connected.
func (p *Panel) welcoming() bool { return p.greeting() && !p.unconfigured() }

// permStep is a permission an entry point needs, and what it is for.
type permStep struct {
	operator.Permission
	use string
}

// permissionSteps are the permissions the configured entry points need, in
// the order the user grants them.
func (p *Panel) permissionSteps() []permStep {
	if p.deps.Permissions == nil {
		return nil
	}
	states := map[string]operator.Permission{}
	for _, s := range p.deps.Permissions() {
		states[s.ID] = s
	}
	uses := map[string]string{}
	for _, e := range operator.Entries(p.cfg) {
		for _, id := range e.Needs {
			if _, ok := uses[id]; !ok {
				uses[id] = permissionUse(id, e.Shortcut.Glyphs())
			}
		}
	}
	// Accessibility takes effect at once; the other two often take a
	// restart, which the step outlasts.
	var steps []permStep
	for _, id := range []string{operator.PermAccessibility, operator.PermInputMonitoring, operator.PermScreenRecording} {
		if s, ok := states[id]; ok && uses[id] != "" {
			steps = append(steps, permStep{s, uses[id]})
		}
	}
	return steps
}

func permissionUse(id, key string) string {
	switch id {
	case operator.PermInputMonitoring:
		return "识别" + key
	case operator.PermAccessibility:
		return key + " 带入选中的文字"
	case operator.PermScreenRecording:
		return key + " 截图识别"
	}
	return key
}

func nextPermission(steps []permStep) *permStep {
	for i := range steps {
		if !steps[i].Granted {
			return &steps[i]
		}
	}
	return nil
}

// restartNames names the granted permissions that take effect once Kami
// starts again.
func restartNames(steps []permStep) string {
	var names []string
	for _, s := range steps {
		if s.Granted && s.Restart {
			names = append(names, "「"+s.Name+"」")
		}
	}
	return strings.Join(names, "和")
}

// stepLabel is what the run button does in the permission step: empty when
// it runs the source as usual.
func (p *Panel) stepLabel() string {
	if !p.welcoming() || strings.TrimSpace(p.source) != "" {
		return ""
	}
	steps := p.permissionSteps()
	switch next := nextPermission(steps); {
	case next != nil:
		return "授权"
	case restartNames(steps) != "":
		return "重新启动"
	}
	return ""
}

// permissionStep takes Enter while the connected reply shows: it asks for
// the next permission, or restarts Kami to apply them. It reports whether
// it took the key.
func (p *Panel) permissionStep() bool {
	if strings.TrimSpace(p.source) != "" {
		return false
	}
	steps := p.permissionSteps()
	if next := nextPermission(steps); next != nil && p.deps.RequestPermission != nil {
		p.deps.RequestPermission(next.ID)
		return true
	}
	if restartNames(steps) != "" && p.deps.Relaunch != nil {
		p.deps.Relaunch()
		return true
	}
	return false
}

// refreshWelcome rewrites the connected reply in place as permissions are
// granted; it does not stream again.
func (p *Panel) refreshWelcome() {
	if !p.welcoming() {
		return
	}
	t := &p.sess.Turns[0]
	if t.Status == session.Streaming {
		return
	}
	if text := p.connectedText(); text != p.said {
		t.Reply, p.said = text, text
	}
	if p.inSetup && setupDone(p.permissionSteps()) {
		p.endSetup()
	}
}

func hostOf(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		return u.Host
	}
	return endpoint
}

// configErr is what is wrong with config.json on disk now.
func (p *Panel) configErr() error {
	if p.deps.Store == nil {
		return nil
	}
	return p.deps.Store.Snapshot().Err
}

// problemsOf lists what is wrong with config.json, one line each.
func problemsOf(err error) []config.Problem {
	var ce *config.Error
	if errors.As(err, &ce) && len(ce.Problems) > 0 {
		return ce.Problems
	}
	return []config.Problem{{Message: err.Error()}}
}

// fixable reports whether a failure is the configuration's, which a coding
// agent can look into.
func fixable(err error) bool {
	var le *llm.Error
	return errors.As(err, &le) && le.Kind == llm.KindConfig && le != errNoModel
}
