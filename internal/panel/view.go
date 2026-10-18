package panel

import (
	"errors"
	"fmt"
	"math"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/markdown/parser"

	"github.com/Umupa/cida-next/internal/design"
	"github.com/Umupa/cida-next/internal/llm"
	"github.com/Umupa/cida-next/internal/operator"
	"github.com/Umupa/cida-next/internal/plaintext"
	"github.com/Umupa/cida-next/internal/session"
)

func (p *Panel) view(c *ui.Context) {
	pal := design.For(c.Theme().Dark)
	c.Root().Background(ui.Transparent)
	c.OnShortcut(ui.Cmd, ui.KeyPeriod, p.stop)
	c.OnShortcut(ui.Cmd, ui.KeyN, p.reset)
	// Cmd-C copies the latest reply, unless the focused input has a
	// selection, which it copies itself.
	c.OnShortcut(ui.Cmd, ui.KeyC, func() { p.copyTurn(-1) })

	maxHeight := float32(p.place.maxHeight)
	if maxHeight <= 0 {
		maxHeight = math.MaxFloat32
	}
	// The panel takes its own height, never the window's: the window
	// follows it, so a window still too short must not squeeze it.
	panel := ui.Column(c).Width(design.PanelWidth).MaxHeight(maxHeight).Shrink(0).
		Background(pal.Surface).Radius(design.RadiusPanel).Border(1, pal.PanelEdge).Clip().
		Children(func() {
			p.sourceView(c, pal)
			ui.Divider(c).Background(pal.Border)
			p.modeBar(c, pal)
			if p.notice != "" {
				ui.Divider(c).Background(pal.Border)
				ui.Text(c, p.notice).FontSize(design.FontSecondary).TextColor(pal.TextMuted).
					PaddingX(design.SpaceWindowX).PaddingY(14)
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
	if p.height != 0 {
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
		Lines(1, p.sourceLines()).
		Placeholder("输入或粘贴文字，⏎ 执行").Label("原文").
		FontSize(design.FontBody).LineHeight(design.LineHeightSource).TextColor(pal.Text).
		PaddingX(design.SpaceWindowX).PaddingY(design.SpaceEntryY).
		TextRanges(p.commandRanges(c, pal)...).
		OnTextKey(func(k ui.TextKey) bool {
			switch {
			case k.Key == ui.KeyEnter && k.Mods == 0:
				p.source = k.Text()
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
	start, end := in.TextSelection()
	p.sourceSelected = start != end
}

// stale reports a result that no longer matches the source or the mode: it
// stays until the user runs again, never refreshed on its own (F5.7).
func (p *Panel) stale() bool {
	if p.sess == nil || p.sess.Busy() || p.sess.Operator || len(p.modes) == 0 {
		return false
	}
	return p.sess.Turns[0].Ask != p.source || p.sess.Mode.ID != p.modes[p.modeIdx].ID
}

// sourceLines is how many lines the source grows to before it scrolls:
// 30% of the visible height.
func (p *Panel) sourceLines() int {
	visible := float32(p.place.maxHeight) / design.PanelMaxRatio
	line := float32(design.FontBody * design.LineHeightSource)
	return max(3, int(visible*design.SourceMaxRatio/line))
}

func (p *Panel) cycleMode(step int) {
	if n := len(p.modes); n > 0 {
		p.modeIdx = (p.modeIdx + step + n) % n
	}
}

func (p *Panel) modeBar(c *ui.Context, pal *design.Palette) {
	ui.Row(c).Height(design.ControlBarHeight).Shrink(0).PaddingX(design.SpaceWindowX - 10).Gap(2).Children(func() {
		if p.operatorActive() {
			p.operatorPill(c, pal)
			ui.Spacer(c)
			p.runButton(c, pal)
			return
		}
		ui.ScrollHorizontal(c).Shrink(1).Children(func() {
			ui.Row(c).Gap(2).Children(func() {
				for i, m := range p.modes {
					name := m.Name
					if m.ID == "translate" && p.cfg != nil {
						name = "翻译成 " + p.cfg.ForeignLanguage
					}
					chip := ui.Box(c.Key(m.ID)).PaddingX(10).PaddingY(5).Radius(design.RadiusChip).
						Children(func() {
							t := ui.Text(c, name).FontSize(design.FontSecondary)
							if i == p.modeIdx {
								t.TextColor(pal.Text).FontWeight(600)
							} else {
								t.TextColor(pal.TextMuted)
							}
						})
					if i == p.modeIdx {
						chip.Background(pal.SurfaceDim)
					}
					if chip.Clicked() {
						p.modeIdx = i
						p.sourceIn.Focus()
					}
				}
			})
		})
		ui.Spacer(c)
		ui.Text(c, "⇥ 切换").FontSize(design.FontCaption).TextColor(pal.Hint)
		p.runButton(c, pal)
	})
}

func (p *Panel) runButton(c *ui.Context, pal *design.Palette) {
	run := ui.Box(c.Key("run")).PaddingX(12).PaddingY(5).Radius(design.RadiusChip).Background(pal.Accent).Children(func() {
		ui.Text(c, "执行 ⏎").FontSize(design.FontSecondary).TextColor(pal.AccentText)
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
	result := ui.Scroll(c.Key("result")).TrackScroll(&p.result).Shrink(1).MinHeight(0)
	p.sparkle(c, result, pal)
	result.
		Background(pal.SurfacePaper).PaddingX(design.SpaceWindowX).PaddingY(design.SpaceResultY).Gap(18).
		Children(func() {
			for i := range p.sess.Turns {
				p.turnView(c.Key(i), pal, i)
			}
		})
}

func (p *Panel) turnView(c *ui.Context, pal *design.Palette, i int) {
	t := &p.sess.Turns[i]
	ui.Column(c).Gap(8).Children(func() {
		if i > 0 {
			ui.Text(c, t.Ask).FontSize(design.FontSecondary).TextColor(pal.TextMuted).
				PaddingStart(10).BorderStartWidth(2).BorderColor(pal.Border).Selectable()
		}
		switch {
		case t.Reply == "" && t.Status == session.Streaming:
			ui.Text(c, "正在生成…").FontSize(design.FontSecondary).TextColor(pal.TextFaint)
		case t.Status == session.Streaming && p.sess.Operator:
			markdown.Stream(c, operator.Visible(t.Reply), &p.md)
		case t.Status == session.Streaming:
			markdown.Stream(c, t.Reply, &p.md)
		default:
			markdown.View(c, t.Reply, &p.md)
		}
		if i == len(p.sess.Turns)-1 {
			p.turnFooter(c, pal, i)
		} else if t.Reply != "" {
			ui.Row(c).Children(func() {
				ui.Spacer(c)
				if ui.Text(c, "复制").FontSize(design.FontCaption).TextColor(pal.TextMuted).Clicked() {
					p.copyTurn(i)
				}
			})
		}
	})
}

// turnFooter shows how the latest turn ended and what to do next.
func (p *Panel) turnFooter(c *ui.Context, pal *design.Palette, i int) {
	t := &p.sess.Turns[i]
	ui.Row(c).Gap(12).Children(func() {
		status := map[session.Status]string{
			session.Streaming: "生成中 · ⌘. 停止",
			session.Done:      "完成",
			session.Stopped:   "已停止",
			session.Failed:    "失败",
		}[t.Status]
		if p.sess.Operator {
			status = "Operator · " + status
		}
		if t.Local {
			status = "cida-next"
		} else if t.Model != "" {
			status += " · " + t.Model
		}
		if t.Dropped > 0 {
			status += fmt.Sprintf(" · 为放进上下文省略了之前 %d 轮", t.Dropped)
		}
		if t.Parts > 1 {
			status += fmt.Sprintf(" · 原文较长，分 %d 段处理", t.Parts)
		}
		ui.Text(c, status).FontSize(design.FontCaption).TextColor(pal.TextMuted)
		if p.stale() {
			ui.Text(c, "原文或模式已改，⏎ 重新执行").FontSize(design.FontCaption).TextColor(pal.Accent)
		}
		ui.Spacer(c)
		if t.Undoable {
			if ui.Text(c, "撤销").FontSize(design.FontCaption).TextColor(pal.Operator).Clicked() {
				p.undoOperator()
			}
		}
		if t.Status != session.Streaming && !t.Local {
			if ui.Text(c, "重新生成").FontSize(design.FontCaption).TextColor(pal.TextControl).Clicked() {
				p.retry()
			}
			if t.Reply != "" {
				ui.Text(c, "复制为…").FontSize(design.FontCaption).TextColor(pal.TextControl).Menu(func(m *ui.Menu) {
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
			}
			if t.Reply != "" && ui.Text(c, "复制 ⌘C").FontSize(design.FontCaption).TextColor(pal.TextControl).Clicked() {
				p.copyTurn(i)
			}
		}
	})
	if t.Status == session.Failed {
		title, next := failureText(t.Err)
		ui.Column(c).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(design.FontSecondary).TextColor(pal.Text)
			ui.Text(c, next).FontSize(design.FontSecondary).TextColor(pal.TextMuted)
		})
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
	ui.Row(c).Shrink(0).PaddingX(design.SpaceWindowX).PaddingY(12).Gap(12).Children(func() {
		in := ui.TextAreaBase(c.Key("follow"), &p.follow).Bind(&p.followIn).Grow(1).
			Lines(1, 4).Placeholder(p.followPlaceholder()).Label("追问").
			FontSize(design.FontBody).LineHeight(design.LineHeightSource).TextColor(pal.Text).
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
		ui.Text(c, "⏎ 发送").FontSize(design.FontCaption).TextColor(pal.Hint)
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
	}
}
