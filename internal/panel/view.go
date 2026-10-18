package panel

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/design"
	"github.com/Umupa/cida-next/internal/llm"
	"github.com/Umupa/cida-next/internal/operator"
	"github.com/Umupa/cida-next/internal/plaintext"
	"github.com/Umupa/cida-next/internal/session"
)

func (p *Panel) view(c *ui.Context) {
	theme, fontName, size, blur := "", "", float32(0), true
	if p.cfg != nil {
		theme, fontName, size, blur = p.cfg.Theme, p.cfg.Font, p.cfg.FontSize, p.cfg.Background != "solid"
	}
	dark := c.Theme().Dark
	pal := design.For(theme, dark)
	if preview := p.previewTheme(pal); preview != "" {
		pal = design.For(preview, dark)
	}
	if preview := p.previewFont(); preview != "" {
		fontName = preview
	}
	p.pal, p.sz, p.font = pal, design.SizesFor(size), design.FontNamed(fontName).Use()
	c.SetTheme(design.UITheme(*c.Theme(), pal, p.font.Family, p.sz.Body))
	if key := (mdKey{pal, p.font.Name, p.sz.Body}); p.mdFor != key {
		p.md.Theme, p.mdFor = design.Markdown(pal, p.font, p.sz.Body), key
	}
	c.Root().Background(ui.Transparent)
	c.OnShortcut(ui.Cmd, ui.KeyEqual, func() { p.stepFontSize(1) })
	c.OnShortcut(ui.Cmd, ui.KeyMinus, func() { p.stepFontSize(-1) })
	c.OnShortcut(ui.Cmd, ui.Key0, func() { p.stepFontSize(0) })
	c.OnShortcut(ui.Cmd, ui.KeyP, p.togglePin)
	background := pal.Background
	if blur && c.Vibrancy() {
		// The desktop shows blurred through the panel's tint.
		background = background.Alpha(0.78)
	}
	c.OnShortcut(ui.Cmd, ui.KeyPeriod, p.stop)
	c.OnShortcut(ui.Cmd, ui.KeyN, p.reset)
	c.OnShortcut(ui.Cmd, ui.KeyJ, func() { p.switchInput(c) })
	// Cmd-C copies the latest reply, unless the focused input has a
	// selection, which it copies itself.
	c.OnShortcut(ui.Cmd, ui.KeyC, func() { p.copyTurn(-1) })

	maxHeight := float32(p.place.maxHeight)
	if maxHeight <= 0 {
		maxHeight = math.MaxFloat32
	}
	// The panel takes its own height, never the window's: the window
	// follows it, so a window still too short must not squeeze it.
	// The panel fills the window across: the window's width is the user's.
	panel := ui.Column(c).FillWidth().MaxHeight(maxHeight).Shrink(0).
		Background(background).Radius(design.RadiusPanel).Border(1, pal.Edge).Clip()
	if p.fixedHeight > 0 && p.sess != nil {
		panel.Height(float32(p.fixedHeight))
	}
	panel.Children(func() {
		p.sourceView(c, pal)
		p.menuView(c, pal)
		p.splitter(c, pal)
		p.modeBar(c, pal)
		if p.notice != "" {
			ui.Divider(c).Background(pal.Border)
			ui.Text(c, p.notice).FontSize(p.sz.Label).TextColor(pal.Muted).
				PaddingX(design.SpaceX).PaddingY(12)
		}
		if p.sess != nil {
			ui.Divider(c).Background(pal.Border)
			p.resultView(c, pal)
			ui.Divider(c).Background(pal.Border)
			p.followView(c, pal)
		}
	})
	p.fit(panel)
}

// fit makes the window follow the height the panel was laid out at in the
// last frame, easing over 150 ms (F1.3); the first fit is immediate, so the
// panel appears at once. The window is transparent, so while it eases the
// panel is cut off at its bottom or has nothing below it.
func (p *Panel) fit(panel ui.Element) {
	target := float32(math.Ceil(float64(panel.Bounds().H)))
	if target <= 0 {
		return
	}
	h := int(target)
	// Following the divider or the user's size is immediate; growing with
	// a reply eases.
	if p.height != 0 && !p.split.active && p.fixedHeight == 0 {
		h = int(math.Ceil(float64(panel.Animate("height", target, design.MotionHeight))))
	}
	if h == p.height {
		return
	}
	p.height = h
	p.resize(p.place.bounds(h))
}

func (p *Panel) sourceView(c *ui.Context, pal *design.Palette) {
	// Only the result shrinks when the panel reaches its height limit.
	in := ui.TextAreaBase(c.Key("source"), &p.source).Bind(&p.sourceIn).Shrink(0).
		Placeholder("输入或粘贴文字，/ 查看命令").Label("原文").
		FontSize(p.sz.Source).LineHeight(design.LineHeightSource).TextColor(pal.Bright).
		PaddingX(design.SpaceX).PaddingY(design.SpaceSourceY).
		TextRanges(p.commandRanges(c, pal)...).
		OnTextKey(func(k ui.TextKey) bool {
			p.source = k.Text()
			if p.menuKey(k, p.pal) {
				return true
			}
			switch {
			case k.Key == ui.KeyEnter && k.Mods == 0:
				p.run()
				return true
			case k.Key == ui.KeyTab && k.Mods == 0:
				p.cycleMode(1)
				return true
			case k.Key == ui.KeyTab && k.Mods == ui.Shift:
				p.cycleMode(-1)
				return true
			case k.Key == ui.KeyC && k.Mods == ui.Cmd && !p.sourceSelected:
				p.copyTurn(-1)
				return true
			}
			return false
		})
	if p.sourceHeight > 0 {
		in.Height(p.sourceHeight)
	} else {
		in.Lines(1, p.sourceLines())
	}
	p.sourceBox = in.Bounds()
	if p.caretToEnd {
		n := utf8.RuneCountInString(p.source)
		in.SetTextSelection(n, n)
		p.caretToEnd = false
	}
	start, end := in.TextSelection()
	p.sourceSelected = start != end
}

// switchInput moves the keyboard between the source and the follow-up
// (Cmd-J).
func (p *Panel) switchInput(c *ui.Context) {
	if p.sess != nil && !p.followIn.Focused(c) {
		p.followIn.Focus()
		return
	}
	p.sourceIn.Focus()
}

// translateTarget is where translation goes for the source as typed: into
// the foreign language from text mostly in Chinese, into my language from
// anything else. It only labels the mode; the model decides.
func (p *Panel) translateTarget() string {
	if p.cfg == nil || strings.TrimSpace(p.source) == "" {
		return ""
	}
	var han, letters int
	for _, r := range p.source {
		switch {
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.IsLetter(r):
			letters++
		}
	}
	mine := p.cfg.MyLanguage
	chineseIsMine := strings.Contains(mine, "中文") || strings.Contains(mine, "汉") || strings.Contains(strings.ToLower(mine), "chinese")
	if chineseIsMine && han*2 >= letters {
		return p.cfg.ForeignLanguage
	}
	return mine
}

// stale reports a result that no longer matches the source or the mode: it
// stays until the user runs again, never refreshed on its own (F5.7).
func (p *Panel) stale() bool {
	if p.sess == nil || p.sess.Busy() || p.sess.Operator || len(p.modes) == 0 {
		return false
	}
	return p.sess.Turns[0].Ask != p.source || p.sess.Mode.ID != p.modes[p.modeIdx].ID
}

// sourceLines is how many lines the source grows to before it scrolls: 30%
// of the visible height on its own, three lines once there is a result, which
// is what the user reads; the divider gives it more.
func (p *Panel) sourceLines() int {
	if p.sess != nil {
		return 3
	}
	visible := float32(p.place.maxHeight) / design.PanelMaxRatio
	line := float32(p.sz.Source * design.LineHeightSource)
	return max(3, int(visible*design.SourceMaxRatio/line))
}

func (p *Panel) cycleMode(step int) {
	if n := len(p.modes); n > 0 {
		p.modeIdx = (p.modeIdx + step + n) % n
	}
}

func (p *Panel) modeBar(c *ui.Context, pal *design.Palette) {
	// The bar is where the panel is held to move it.
	ui.Row(c).Height(design.ControlBarHeight).Shrink(0).PaddingX(design.SpaceX - 8).Gap(2).DragWindow().Children(func() {
		if p.operatorActive() {
			p.operatorPill(c, pal)
			ui.Spacer(c)
			p.pinButton(c, pal)
			p.runButton(c, pal)
			return
		}
		ui.ScrollHorizontal(c).Shrink(1).Children(func() {
			ui.Row(c).Gap(2).Children(func() {
				for i, m := range p.modes {
					name := m.Name
					if m.ID == "translate" && i == p.modeIdx {
						if to := p.translateTarget(); to != "" {
							name += " → " + to
						}
					}
					chip := ui.Box(c.Key(m.ID)).PaddingX(8).PaddingY(4).Radius(design.RadiusChip)
					chip.Children(func() {
						t := ui.Text(c, name).FontSize(p.sz.Label)
						switch {
						case i == p.modeIdx:
							t.TextColor(pal.Accent).FontWeight(700)
						case chip.Hovered():
							t.TextColor(pal.Text)
						default:
							t.TextColor(pal.Muted)
						}
					})
					if i == p.modeIdx {
						chip.Background(pal.Raised)
					}
					if chip.Clicked() {
						p.modeIdx = i
						p.sourceIn.Focus()
					}
				}
			})
		})
		ui.Spacer(c)
		p.pinButton(c, pal)
		p.themeButton(c, pal)
		p.kbd(c, pal, "tab", "模式")
		p.runButton(c, pal)
	})
}

// kbd shows a key and what it does, as monocode's hints: the key in a chip.
func (p *Panel) kbd(c *ui.Context, pal *design.Palette, key, does string) {
	ui.Row(c).Gap(5).MarginEnd(10).Children(func() {
		ui.Text(c, key).FontSize(p.sz.Caption).TextColor(pal.Muted).
			PaddingX(4).Radius(design.RadiusChip).Border(1, pal.Border)
		ui.Text(c, does).FontSize(p.sz.Caption).TextColor(pal.Faint)
	})
}

var pinIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><path d="M9.8 2.2l4 4-2.3.9-2.6 2.6.3 2.8-1.2 1.2-2.4-3.5-3.2 3.2M5.6 7.9L2.1 5.5l1.2-1.2 2.8.3 2.6-2.6z"/></svg>`))

// pinButton keeps the panel up while other apps have the keyboard (Cmd-P).
func (p *Panel) pinButton(c *ui.Context, pal *design.Palette) {
	label := "固定面板 ⌘P"
	if p.pinned {
		label = "取消固定 ⌘P"
	}
	b := ui.Box(c.Key("pin")).Padding(4).MarginEnd(6).Radius(design.RadiusChip).Label(label).Tooltip(label)
	col := pal.Faint
	switch {
	case p.pinned:
		col = pal.Accent
		b.Background(pal.Raised)
	case b.Hovered():
		col = pal.Text
	}
	b.Children(func() { ui.Icon(c, pinIcon).Size(p.sz.Label+1, p.sz.Label+1).TextColor(col) })
	if b.Clicked() {
		p.togglePin()
	}
}

// themeButton opens the theme picker.
func (p *Panel) themeButton(c *ui.Context, pal *design.Palette) {
	name := "auto"
	if p.cfg != nil {
		name = p.cfg.Theme
	}
	b := ui.Row(c.Key("theme")).Gap(5).PaddingX(6).PaddingY(3).MarginEnd(10).Radius(design.RadiusChip).Label("切换主题")
	b.Children(func() {
		ui.Text(c, "◐").FontSize(p.sz.Label).TextColor(pal.Accent)
		col := pal.Faint
		if b.Hovered() {
			col = pal.Text
		}
		ui.Text(c, name).FontSize(p.sz.Caption).TextColor(col)
	})
	if b.Clicked() {
		p.source = themeCommand + " "
		p.caretToEnd = true
		p.sourceIn.Focus()
	}
}

func (p *Panel) runButton(c *ui.Context, pal *design.Palette) {
	run := ui.Row(c.Key("run")).Gap(6).PaddingX(10).PaddingY(4).Radius(design.RadiusChip).Background(pal.Accent)
	run.Children(func() {
		ui.Text(c, "⏎").FontSize(p.sz.Label).TextColor(pal.AccentText)
		ui.Text(c, "执行").FontSize(p.sz.Label).FontWeight(700).TextColor(pal.AccentText)
	})
	if run.Clicked() {
		p.run()
	}
}

func (p *Panel) resultView(c *ui.Context, pal *design.Palette) {
	// A long reply is read from its start: the result follows the end only
	// once the user has scrolled down to it (Cida's panel.md §二).
	if p.sess.Busy() && p.result.Y > 0 && p.result.Y >= p.result.MaxY-1 {
		p.result.Y = math.MaxFloat32
	}
	result := ui.Scroll(c.Key("result")).TrackScroll(&p.result).Grow(1).Shrink(1).MinHeight(0)
	p.sparkle(c, result, pal)
	result.PaddingX(design.SpaceX).PaddingY(design.SpaceResultY).Gap(design.SpaceTurns).
		Children(func() {
			for i := range p.sess.Turns {
				p.turnView(c.Key(i), pal, i)
			}
		})
}

func (p *Panel) turnView(c *ui.Context, pal *design.Palette, i int) {
	t := &p.sess.Turns[i]
	turn := ui.Column(c).Gap(10)
	turn.Children(func() {
		if i > 0 {
			// A follow-up, as a prompt line in a terminal.
			ui.Row(c).Gap(8).AlignItems(ui.Start).Children(func() {
				ui.Text(c, "❯").FontSize(p.sz.Body).TextColor(pal.Accent).FontWeight(700)
				ui.Text(c, t.Ask).FontSize(p.sz.Body).TextColor(pal.Bright).Grow(1).Selectable()
			})
		}
		switch {
		case t.Reply == "" && t.Status == session.Streaming:
			p.thinking(c, pal)
		case t.Status == session.Streaming && p.sess.Operator:
			markdown.Stream(c, operator.Visible(t.Reply), &p.md)
		case t.Status == session.Streaming:
			markdown.Stream(c, t.Reply, &p.md)
		default:
			markdown.View(c, t.Reply, &p.md)
		}
		last := i == len(p.sess.Turns)-1
		if last || turn.Hovered() {
			p.turnFooter(c, pal, i)
		}
	})
}

// thinking shows that a reply is on its way: a pulse, as monocode's.
func (p *Panel) thinking(c *ui.Context, pal *design.Palette) {
	row := ui.Row(c.Key("thinking")).Gap(8)
	pulse := row.Loop("pulse", design.MotionThinking, ui.EaseInOut)
	alpha := 0.35 + 0.55*float32(0.5-0.5*math.Cos(2*math.Pi*float64(pulse)))
	row.Children(func() {
		ui.Text(c, "●").FontSize(p.sz.Caption).TextColor(pal.Accent.Alpha(alpha))
		ui.Text(c, "思考中").FontSize(p.sz.Label).TextColor(pal.Muted.Alpha(alpha + 0.1))
	})
}

// turnFooter shows how a turn ended and what can be done with it: always
// under the latest turn, on hover under the others.
func (p *Panel) turnFooter(c *ui.Context, pal *design.Palette, i int) {
	t := &p.sess.Turns[i]
	last := i == len(p.sess.Turns)-1
	ui.Row(c.Key("footer")).Gap(10).Children(func() {
		var parts []string
		switch {
		case t.Local:
			parts = append(parts, "cida-next")
		default:
			if p.sess.Operator {
				parts = append(parts, "operator")
			}
			parts = append(parts, map[session.Status]string{
				session.Streaming: "生成中 ⌘. 停止",
				session.Done:      "完成",
				session.Stopped:   "已停止",
				session.Failed:    "失败",
			}[t.Status])
			if t.Model != "" {
				parts = append(parts, t.Model)
			}
		}
		if t.Dropped > 0 {
			parts = append(parts, fmt.Sprintf("省略了之前 %d 轮", t.Dropped))
		}
		if t.Parts > 1 {
			parts = append(parts, fmt.Sprintf("分 %d 段处理", t.Parts))
		}
		ui.Text(c, strings.Join(parts, " · ")).FontSize(p.sz.Caption).TextColor(pal.Faint)
		if last && p.stale() {
			ui.Text(c, "原文或模式已改 ⏎ 重新执行").FontSize(p.sz.Caption).TextColor(pal.Sparkle)
		}
		ui.Spacer(c)
		if t.Undoable {
			p.action(c, pal, "撤销", pal.Operator, func() { p.undoOperator() })
		}
		if t.Status == session.Streaming || t.Reply == "" {
			return
		}
		if last && !t.Local {
			p.action(c, pal, "重新生成", pal.Muted, p.retry)
		}
		ui.Text(c, "复制为…").FontSize(p.sz.Caption).TextColor(pal.Muted).Menu(func(m *ui.Menu) {
			if m.Item("纯文本").Chosen() {
				p.writeClipboard(plaintext.FromMarkdown(t.Reply))
			}
			if m.Item("富文本（飞书、Notion、邮件）").Chosen() {
				p.writeRich(parser.Parse(t.Reply).HTML(), plaintext.FromMarkdown(t.Reply))
			}
			if m.Item("Markdown").Chosen() {
				p.writeClipboard(t.Reply)
			}
		})
		label := "复制"
		if last {
			label = "复制 ⌘C"
		}
		if p.copiedTurn == i && c.Now().Sub(p.copiedAt) < design.MotionCopiedHold {
			label = "✓ 已复制"
			c.After(design.MotionCopiedHold)
		}
		p.action(c, pal, label, pal.Muted, func() { p.copyTurn(i) })
	})
	if t.Status == session.Failed {
		title, next := failureText(t.Err)
		ui.Column(c).Gap(2).PaddingStart(10).BorderStartWidth(2).BorderColor(pal.Danger).Children(func() {
			ui.Text(c, title).FontSize(p.sz.Label).TextColor(pal.Bright)
			ui.Text(c, next).FontSize(p.sz.Label).TextColor(pal.Muted)
		})
	}
}

// action is a text button of a turn's footer, brighter on hover.
func (p *Panel) action(c *ui.Context, pal *design.Palette, label string, color ui.Color, fn func()) {
	t := ui.Text(c.Key(label), label).FontSize(p.sz.Caption)
	if t.Hovered() {
		color = pal.Bright
	}
	t.TextColor(color)
	if t.Clicked() {
		fn()
	}
}

// failureText says what failed and what to do, by what the user can do
// about it (Cida's panel.md §四).
func failureText(err error) (title, next string) {
	var le *llm.Error
	if !errors.As(err, &le) {
		return "请求没有完成。", "重试 ⏎"
	}
	switch le.Kind {
	case llm.KindTimeout:
		return "模型服务没有及时回应。", "稍后重试 ⏎"
	case llm.KindOffline:
		return "连不上模型服务。", "检查网络后重试 ⏎"
	case llm.KindLimited:
		return "模型服务暂时限制了请求。", "稍后重试 ⏎"
	case llm.KindTLS:
		return "和模型服务的安全连接失败。", "检查网络或代理后重试 ⏎"
	case llm.KindConfig:
		if le == errNoModel {
			return "还没有配置模型。", "输入 /operator，按提示把配置交给 Claude Code 等 Coding Agent。"
		}
		return "模型服务拒绝了这个请求。", "输入 /operator 配置模型，复制提示词交给 Coding Agent 检查。"
	case llm.KindTooLong:
		return "内容超过了模型能处理的长度。", "缩短原文后重试 ⏎"
	}
	return "请求没有完成。", "重试 ⏎"
}

func (p *Panel) followView(c *ui.Context, pal *design.Palette) {
	ui.Row(c).Shrink(0).PaddingX(design.SpaceX).PaddingY(11).Gap(8).AlignItems(ui.Start).Children(func() {
		prompt := pal.Faint
		if p.followIn.Focused(c) {
			prompt = pal.Accent
		}
		ui.Text(c, "❯").FontSize(p.sz.Body).FontWeight(700).TextColor(prompt)
		in := ui.TextAreaBase(c.Key("follow"), &p.follow).Bind(&p.followIn).Grow(1).
			Lines(1, 4).Placeholder(p.followPlaceholder()).Label("追问").
			FontSize(p.sz.Body).LineHeight(design.LineHeightSource).TextColor(pal.Bright).
			OnTextKey(func(k ui.TextKey) bool {
				switch {
				case k.Key == ui.KeyEnter && k.Mods == 0:
					p.follow = k.Text()
					p.send()
					return true
				case k.Key == ui.KeyC && k.Mods == ui.Cmd && !p.followSelected:
					p.copyTurn(-1)
					return true
				case k.Key == ui.KeyZ && k.Mods == ui.Cmd && k.Text() == "":
					return p.undoOperator()
				}
				return false
			})
		start, end := in.TextSelection()
		p.followSelected = start != end
		p.kbd(c, pal, "⌘J", "切换")
		p.kbd(c, pal, "⏎", "发送")
	})
}

func (p *Panel) followPlaceholder() string {
	if p.sess != nil && p.sess.Operator {
		return "继续让 Operator 查看或修改设置…"
	}
	return "追问…"
}

// copyTurn copies a turn's reply as Markdown; -1 is the latest turn.
func (p *Panel) copyTurn(i int) {
	if p.sess == nil {
		return
	}
	if i < 0 {
		i = len(p.sess.Turns) - 1
	}
	if reply := p.sess.Turns[i].Reply; reply != "" {
		p.writeClipboard(reply)
		p.copiedTurn, p.copiedAt = i, time.Now()
	}
}

// mdKey is what the Markdown theme depends on.
type mdKey struct {
	pal  *design.Palette
	font string
	size float32
}

// stepFontSize makes the text a point larger or smaller, or, at 0, the
// default size, and keeps it in config.json (Cmd-=, Cmd--, Cmd-0).
func (p *Panel) stepFontSize(step float32) {
	size := p.sz.Body + step
	if step == 0 {
		size = design.FontBody
	}
	size = max(design.MinFontSize, min(size, design.MaxFontSize))
	p.setConfig("fontSize", size, func(c *config.Config) { c.FontSize = size })
}

// togglePin keeps the panel up while other apps have the keyboard, or lets
// it hide with them again (Cmd-P).
func (p *Panel) togglePin() {
	p.pinned = !p.pinned
	if p.win != nil {
		p.win.SetAlwaysOnTop(p.pinned)
	}
}

// setConfig writes one setting through operator mode's edits, and shows it
// at once, before the store reloads the file.
func (p *Panel) setConfig(path string, value any, apply func(*config.Config)) {
	data, _ := json.Marshal(value)
	if p.deps.Operator.Apply != nil {
		if _, err := p.deps.Operator.Apply([]config.Op{{Path: path, Value: data}}); err != nil {
			p.notice = "没有保存：" + err.Error()
			return
		}
	}
	if p.cfg != nil {
		cfg := *p.cfg
		apply(&cfg)
		p.cfg = &cfg
	}
}
