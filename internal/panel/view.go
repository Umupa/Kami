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

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/design"
	"github.com/Umupa/Kami/internal/llm"
	"github.com/Umupa/Kami/internal/operator"
	"github.com/Umupa/Kami/internal/plaintext"
	"github.com/Umupa/Kami/internal/session"
)

func (p *Panel) view(c *ui.Context) {
	theme, size, blur := "", float32(0), true
	if p.cfg != nil {
		theme, size, blur = p.cfg.Theme, p.cfg.FontSize, p.cfg.Background != "solid"
	}
	dark := c.Theme().Dark
	pal := design.For(theme, dark)
	if preview := p.previewTheme(pal); preview != "" {
		pal = design.For(preview, dark)
	}
	p.pal, p.sz = pal, design.SizesFor(size)
	c.SetTheme(design.UITheme(*c.Theme(), pal, p.sz.Body))
	if key := (mdKey{pal, p.sz.Body}); p.mdFor != key {
		p.md.Theme, p.mdFor = design.Markdown(pal, p.sz.Body), key
	}
	c.Root().Background(ui.Transparent)
	c.OnShortcut(ui.Cmd, ui.KeyEqual, func() { p.stepFontSize(1) })
	c.OnShortcut(ui.Cmd, ui.KeyMinus, func() { p.stepFontSize(-1) })
	c.OnShortcut(ui.Cmd, ui.Key0, func() { p.stepFontSize(0) })
	c.OnShortcut(ui.Cmd|ui.Shift, ui.KeyP, p.togglePin)
	background := pal.Background
	if blur && c.Vibrancy() {
		// The desktop shows blurred through the panel's tint.
		background = background.Alpha(0.78)
	}
	if p.noteMode() {
		c.OnShortcut(ui.Cmd, ui.KeyN, p.newNote)
		c.OnShortcut(ui.Cmd, ui.KeyP, p.toggleList)
		c.OnShortcut(ui.Cmd, ui.KeyBracketLeft, func() { p.stepNote(-1) })
		c.OnShortcut(ui.Cmd, ui.KeyBracketRight, func() { p.stepNote(1) })
	} else {
		c.OnShortcut(ui.Cmd, ui.KeyPeriod, p.stop)
		c.OnShortcut(ui.Cmd, ui.KeyN, p.reset)
		c.OnShortcut(ui.Cmd, ui.KeyJ, func() { p.switchInput(c) })
		// Cmd-C copies the latest reply, unless the focused input has a
		// selection, which it copies itself.
		c.OnShortcut(ui.Cmd, ui.KeyC, func() { p.copyTurn(-1) })
	}

	maxHeight := float32(p.place.maxHeight)
	if maxHeight <= 0 {
		maxHeight = math.MaxFloat32
	}
	_, windowH := c.Size()
	// The panel is two boxes. The outer one fills the window across, the
	// width being the user's, and at least down to the window's bottom, so
	// that in the one frame before a shrinking window catches up the
	// panel's background, not an empty strip, shows below the content. The
	// inner one is the content at its own height, which its Draw reads in
	// the very frame it is laid out, and the window follows at once.
	panel := ui.Column(c).FillWidth().MinHeight(min(windowH, maxHeight)).Shrink(0).
		Background(background).Radius(design.RadiusPanel).Border(1, pal.Edge).Clip()
	if p.fixedLayout() {
		panel.Height(float32(p.fixedHeight))
	}
	panel.Children(func() {
		content := ui.Column(c.Key("content")).MaxHeight(maxHeight).Shrink(0)
		if p.fixedLayout() {
			content.Grow(1).Shrink(1).MinHeight(0)
		}
		content.Draw(func(_ *ui.Painter, r ui.Rect) { p.fit(r.H) })
		content.Children(func() {
			if p.noteMode() {
				p.noteView(c, pal)
				ui.Divider(c).Background(pal.Border)
				p.modeBar(c, pal)
				return
			}
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
	})
}

// fit makes the window as tall as the panel's content, measured as it is
// drawn: the window catches up within the frame after, which the outer box
// covers. A height the user set by resizing stays.
func (p *Panel) fit(contentH float32) {
	p.panelH = contentH
	if p.fixedLayout() {
		return
	}
	h := int(math.Ceil(float64(contentH)))
	if h <= 0 || h == p.height {
		return
	}
	p.height = h
	p.resize(p.place.bounds(h))
}

// fixedLayout reports whether the panel keeps the height the user resized
// it to, rather than following its content: with a result or a note, which
// fill it.
func (p *Panel) fixedLayout() bool {
	return p.fixedHeight > 0 && (p.sess != nil || p.noteMode())
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
	if p.sess == nil || p.sess.Busy() || p.sess.Operator || len(p.modes) == 0 || p.noteMode() {
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

// cycleMode moves to the next mode (Tab) or the one before (Shift-Tab),
// notes being the last.
func (p *Panel) cycleMode(step int) {
	n := len(p.modes) + 1
	p.setMode((p.modeIdx + step + n) % n)
}

func (p *Panel) modeBar(c *ui.Context, pal *design.Palette) {
	// The bar is where the panel is held to move it.
	ui.Row(c).Height(design.ControlBarHeight).Shrink(0).PaddingX(design.SpaceX - 8).Gap(2).DragWindow().Children(func() {
		if p.operatorActive() && !p.noteMode() {
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
						p.setMode(i)
						p.sourceIn.Focus()
					}
				}
				p.notesChip(c, pal)
			})
		})
		ui.Spacer(c)
		if p.noteMode() {
			p.noteBar(c, pal)
			p.pinButton(c, pal)
			return
		}
		p.pinButton(c, pal)
		p.themeButton(c, pal)
		p.kbd(c, pal, "tab", "模式")
		p.runButton(c, pal)
	})
}

// notesChip is the tab of notes mode, after the modes.
func (p *Panel) notesChip(c *ui.Context, pal *design.Palette) {
	chip := ui.Box(c.Key("notes")).PaddingX(8).PaddingY(4).Radius(design.RadiusChip).MarginStart(6)
	on := p.noteMode()
	chip.Children(func() {
		t := ui.Text(c, "笔记").FontSize(p.sz.Label)
		switch {
		case on:
			t.TextColor(pal.Accent).FontWeight(700)
		case chip.Hovered():
			t.TextColor(pal.Text)
		default:
			t.TextColor(pal.Muted)
		}
	})
	if on {
		chip.Background(pal.Raised)
	}
	if chip.Clicked() && !on {
		p.setMode(len(p.modes))
	}
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

// pinButton keeps the panel up while other apps have the keyboard (Cmd-Shift-P).
func (p *Panel) pinButton(c *ui.Context, pal *design.Palette) {
	label := "固定面板 ⌘⇧P"
	if p.pinned {
		label = "取消固定 ⌘⇧P"
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
	ui.Box(c.Key("result")).Grow(1).Shrink(1).MinHeight(0).Children(func() {
		// FollowAnimation also offsets rows that fit in mygo's List. Only
		// animate once there is an actual scroll range; Reveal handles the
		// appearance of text while the result still fits.
		// While the window can grow, List measures its intrinsic height
		// from the previous frame. Following that temporary end scrolls
		// upward for one frame before the window catches up.
		if !p.list.FollowEnd && (p.fixedLayout() || p.panelH >= float32(p.place.maxHeight)-0.5) {
			p.list.FollowEnd = true
			p.list.ScrollToEnd()
		}
		p.list.FollowAnimation = nil
		if p.list.FollowEnd && p.list.scroll.MaxY > 0.5 {
			p.list.FollowAnimation = &ui.Smooth
		}
		list := ui.List(c, &p.list.ListState, len(p.sess.Turns), func(i int) {
			p.turnView(c, pal, i)
		}).Fill().PaddingX(design.SpaceX).PaddingY(design.SpaceResultY).Gap(design.SpaceTurns).TrackScroll(&p.list.scroll)
		p.sparkle(c, list, pal)
		if p.list.FollowEnd && !p.list.AtEnd() {
			// Back to the latest, and following it again.
			jump := ui.Box(c.Key("latest")).PaddingX(9).PaddingY(4).Radius(design.RadiusChip).Label("回到底部").
				Background(pal.Raised).Border(1, pal.Border).Attach(ui.AnchorBottom, ui.AnchorBottom).Bottom(10)
			jump.Children(func() {
				ui.Text(c, "↓").FontSize(p.sz.Caption).TextColor(pal.Bright)
			})
			if jump.Clicked() {
				p.list.ScrollToEnd()
			}
		}
	})
}

func (p *Panel) turnView(c *ui.Context, pal *design.Palette, i int) {
	t := &p.sess.Turns[i]
	turn := ui.Column(c).Gap(10)
	if !p.fixedLayout() {
		// List's intrinsic size uses cached row heights. Rebuild after
		// measurement so the auto-sized window grows and shrinks without
		// relying on a scroll animation to schedule the next frame.
		height := ui.Local(turn, "measured-height", func() float32 { return 0 })
		turn.Draw(func(_ *ui.Painter, r ui.Rect) {
			if *height != r.H {
				*height = r.H
				c.Invalidate()
			}
		})
	}
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
		p.turnFooter(c, pal, i, last || turn.Hovered())
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

// turnFooter reserves its height even when the historical turn's actions
// are hidden: scrolling under the pointer must not resize list rows.
func (p *Panel) turnFooter(c *ui.Context, pal *design.Palette, i int, visible bool) {
	t := &p.sess.Turns[i]
	last := i == len(p.sess.Turns)-1
	ui.Row(c.Key("footer")).Gap(2).Height(p.sz.Label + 14).Children(func() {
		if !visible {
			return
		}
		var notes []string
		if t.Dropped > 0 {
			notes = append(notes, fmt.Sprintf("省略了之前 %d 轮", t.Dropped))
		}
		if t.Parts > 1 {
			notes = append(notes, fmt.Sprintf("分 %d 段处理", t.Parts))
		}
		if len(notes) > 0 {
			ui.Text(c, strings.Join(notes, " · ")).FontSize(p.sz.Caption).TextColor(pal.Faint)
		}
		if last && p.stale() {
			ui.Text(c, "原文或模式已改 ⏎ 重新执行").FontSize(p.sz.Caption).TextColor(pal.Sparkle).MarginStart(8)
		}
		ui.Spacer(c)
		if t.Status == session.Streaming || t.Reply == "" {
			return
		}
		if t.Undoable {
			p.iconButton(c, pal, "undo", undoIcon, "撤销这次修改 ⌘Z", pal.Operator, func() { p.undoOperator() })
		}
		if last && !t.Local {
			p.iconButton(c, pal, "retry", retryIcon, "重新生成", pal.Muted, p.retry)
		}
		more := p.iconButton(c, pal, "copy-as", moreIcon, "复制为…", pal.Muted, nil)
		more.Menu(func(m *ui.Menu) {
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
		icon, tip, col := copyIcon, "复制 Markdown", pal.Muted
		if last {
			tip = "复制 Markdown ⌘C"
		}
		if p.copiedTurn == i && c.Now().Sub(p.copiedAt) < design.MotionCopiedHold {
			icon, tip, col = checkIcon, "已复制", pal.Code
			c.After(design.MotionCopiedHold)
		}
		p.iconButton(c, pal, "copy", icon, tip, col, func() { p.copyTurn(i) })
	})
	if t.Status == session.Failed {
		title, next := failureText(t.Err)
		ui.Column(c).Gap(2).PaddingStart(10).BorderStartWidth(2).BorderColor(pal.Danger).Children(func() {
			ui.Text(c, title).FontSize(p.sz.Label).TextColor(pal.Bright)
			ui.Text(c, next).FontSize(p.sz.Label).TextColor(pal.Muted)
		})
	}
}

// Icons of the turn footer, drawn in the text color.
var (
	copyIcon  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4"><rect x="5.5" y="5.5" width="8" height="8" rx="1.6"/><path d="M10.5 3.6V3.2A1.7 1.7 0 0 0 8.8 1.5H4.2A1.7 1.7 0 0 0 2.5 3.2v4.6a1.7 1.7 0 0 0 1.7 1.7h.4"/></svg>`))
	checkIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M3.5 8.5l3 3 6-7"/></svg>`))
	moreIcon  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><path d="M4 6.5l4 4 4-4"/></svg>`))
	retryIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><path d="M2.5 8a5.5 5.5 0 1 0 1.6-3.9"/><path d="M2.5 2.5v3h3"/></svg>`))
	undoIcon  = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><path d="M5.5 3.5L2.5 6.5l3 3"/><path d="M2.5 6.5h7a4 4 0 0 1 0 8H7"/></svg>`))
)

// iconButton is an icon of a turn's footer, with its name as tooltip and
// label; it brightens on hover.
func (p *Panel) iconButton(c *ui.Context, pal *design.Palette, key string, icon *ui.SVG, tip string, col ui.Color, fn func()) ui.Element {
	b := ui.Box(c.Key(key)).Padding(4).Radius(design.RadiusChip).Label(tip).Tooltip(tip)
	if b.Hovered() {
		b.Background(pal.Raised)
		if col == pal.Muted {
			col = pal.Bright
		}
	}
	size := p.sz.Label + 2
	b.Children(func() { ui.Icon(c, icon).Size(size, size).TextColor(col) })
	if fn != nil && b.Clicked() {
		fn()
	}
	return b
}

// failureText groups failures by the next action the user can take.
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
