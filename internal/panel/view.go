package panel

import (
	"errors"
	"math"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/markdown"

	"github.com/Umupa/cida-next/internal/design"
	"github.com/Umupa/cida-next/internal/llm"
	"github.com/Umupa/cida-next/internal/session"
)

func (p *Panel) view(c *ui.Context) {
	pal := design.For(c.Theme().Dark)
	c.Root().Background(ui.Transparent)
	c.OnShortcut(ui.Cmd, ui.KeyPeriod, p.stop)
	c.OnShortcut(ui.Cmd, ui.KeyN, p.reset)
	c.OnShortcut(ui.Cmd, ui.KeyC, p.copyReply)

	maxHeight := float32(p.place.maxHeight)
	if maxHeight <= 0 {
		maxHeight = math.MaxFloat32
	}
	panel := ui.Column(c).Width(design.PanelWidth).MaxHeight(maxHeight).
		Background(pal.Surface).Radius(design.RadiusPanel).Border(1, pal.PanelEdge).Clip().
		Children(func() {
			p.sourceView(c, pal)
			ui.Divider(c).Background(pal.Border)
			p.modeBar(c, pal)
			if p.sess != nil {
				ui.Divider(c).Background(pal.Border)
				p.resultView(c, pal)
				ui.Divider(c).Background(pal.Border)
				p.followView(c, pal)
			}
		})
	p.fit(panel.Bounds())
}

// fit makes the window as tall as the panel was laid out in the last frame.
func (p *Panel) fit(r ui.Rect) {
	h := int(math.Ceil(float64(r.H)))
	if h <= 0 || h == p.height {
		return
	}
	p.height = h
	p.resize(p.place.bounds(h))
}

func (p *Panel) sourceView(c *ui.Context, pal *design.Palette) {
	ui.TextAreaBase(c.Key("source"), &p.source).Bind(&p.sourceIn).
		Lines(1, p.sourceLines()).
		Placeholder("输入或粘贴文字，⏎ 执行").Label("原文").
		FontSize(design.FontBody).LineHeight(design.LineHeightSource).TextColor(pal.Text).
		PaddingX(design.SpaceWindowX).PaddingY(design.SpaceEntryY).
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
			}
			return false
		})
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
	ui.Row(c).Height(design.ControlBarHeight).PaddingX(design.SpaceWindowX - 10).Gap(2).Children(func() {
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
		run := ui.Box(c).PaddingX(12).PaddingY(5).Radius(design.RadiusChip).Background(pal.Accent).Children(func() {
			ui.Text(c, "执行 ⏎").FontSize(design.FontSecondary).TextColor(pal.AccentText)
		})
		if run.Clicked() {
			p.run()
		}
	})
}

func (p *Panel) resultView(c *ui.Context, pal *design.Palette) {
	ui.Scroll(c.Key("result")).TrackScroll(&p.result).Shrink(1).MinHeight(0).
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
		case t.Status == session.Streaming:
			markdown.Stream(c, t.Reply, &p.md)
		default:
			markdown.View(c, t.Reply, &p.md)
		}
		if i == len(p.sess.Turns)-1 {
			p.turnFooter(c, pal, t)
		}
	})
}

// turnFooter shows how the latest turn ended and what to do next.
func (p *Panel) turnFooter(c *ui.Context, pal *design.Palette, t *session.Turn) {
	ui.Row(c).Gap(12).Children(func() {
		status := map[session.Status]string{
			session.Streaming: "生成中 · ⌘. 停止",
			session.Done:      "完成",
			session.Stopped:   "已停止",
			session.Failed:    "失败",
		}[t.Status]
		if t.Model != "" {
			status += " · " + t.Model
		}
		ui.Text(c, status).FontSize(design.FontCaption).TextColor(pal.TextMuted)
		ui.Spacer(c)
		if t.Status != session.Streaming {
			if ui.Text(c, "重新生成").FontSize(design.FontCaption).TextColor(pal.TextControl).Clicked() {
				p.retry()
			}
			if t.Reply != "" && ui.Text(c, "复制 ⌘C").FontSize(design.FontCaption).TextColor(pal.TextControl).Clicked() {
				p.copyReply()
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
			return "还没有配置模型。", "从菜单栏复制配置提示词，交给 Claude Code 等 AI 助手。"
		}
		return "模型服务拒绝了这个请求。", "从菜单栏复制配置提示词，交给 AI 助手检查配置。"
	case llm.KindTooLong:
		return "内容超过了模型能处理的长度。", "缩短原文后重试 ⏎"
	}
	return "请求没有完成。", "重试 ⏎"
}

func (p *Panel) followView(c *ui.Context, pal *design.Palette) {
	ui.Row(c).PaddingX(design.SpaceWindowX).PaddingY(12).Gap(12).Children(func() {
		ui.TextAreaBase(c.Key("follow"), &p.follow).Bind(&p.followIn).Grow(1).
			Lines(1, 4).Placeholder("追问…").Label("追问").
			FontSize(design.FontBody).LineHeight(design.LineHeightSource).TextColor(pal.Text).
			OnTextKey(func(k ui.TextKey) bool {
				if k.Key == ui.KeyEnter && k.Mods == 0 {
					p.follow = k.Text()
					p.send()
					return true
				}
				return false
			})
		ui.Text(c, "⏎ 发送").FontSize(design.FontCaption).TextColor(pal.Hint)
	})
}

// copyReply copies the latest reply as Markdown.
func (p *Panel) copyReply() {
	if p.sess == nil || p.sess.Last().Reply == "" {
		return
	}
	mygo.Clipboard.WriteText(p.sess.Last().Reply)
}
