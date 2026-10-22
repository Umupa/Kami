package panel

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/design"
	"github.com/Umupa/Kami/internal/operator"
)

// A menu shows under the source while a command is typed, as monocode's
// quick composer does: the commands after "/", the topics after
// "/operator", the themes after "/theme". The arrows choose, Tab
// and Enter take, Escape closes. Enter on bare /operator opens the guide
// unless the arrows explicitly chose a topic.
type menuKind int

const (
	menuNone menuKind = iota
	menuCommands
	menuTopics
	menuThemes
)

// item is an entry of a menu.
type item struct {
	key, title, desc string
	color            ui.Color   // the key's, zero for the default
	swatch           []ui.Color // a theme's colors
	take             func(p *Panel, setText func(string))
}

// themeCommand opens the theme picker. Picking a theme needs no model, so
// it is a command of its own rather than an operator request.
const themeCommand = "/theme"

// themeTyped reports whether src is the theme command, with or without a
// query after it.
func themeTyped(src string) bool {
	return strings.HasPrefix(src, themeCommand) && (len(src) == len(themeCommand) || src[len(themeCommand)] == ' ')
}

// menu returns the menu the source calls for, with its items filtered by
// what is typed.
func (p *Panel) menu(pal *design.Palette) (menuKind, []item) {
	kind, items := p.menuItems(pal)
	if len(items) == 0 {
		// Nothing matches what is typed: no menu, the source is a request.
		return menuNone, nil
	}
	return kind, items
}

func (p *Panel) menuItems(pal *design.Palette) (menuKind, []item) {
	src := p.source
	if src != p.menuChosenFor {
		p.menuChosenFor = ""
	}
	if src != p.menuClosed {
		p.menuClosed = ""
	}
	if p.capturedSource() || src == p.menuClosed || strings.Contains(src, "\n") || p.noteMode() {
		return menuNone, nil
	}
	request, isOperator := operator.Leading(src)
	switch {
	case themeTyped(src):
		return menuThemes, p.themeItems(strings.TrimSpace(src[len(themeCommand):]))
	case isOperator:
		if strings.ContainsAny(request, " \t") {
			return menuNone, nil
		}
		return menuTopics, p.topicItems(request)
	case strings.HasPrefix(src, "/") && !strings.ContainsAny(src, " \t"):
		return menuCommands, p.commandItems(src[1:], pal)
	}
	return menuNone, nil
}

func (p *Panel) commandItems(query string, pal *design.Palette) []item {
	all := []item{{key: "/operator", title: "Operator", desc: "用一句话查看或修改 Kami 的设置", color: pal.Operator,
		take: func(p *Panel, set func(string)) { set(operator.Command + " "); p.caretToEnd = true }},
		{key: themeCommand, title: "主题",
			take: func(p *Panel, set func(string)) { set(themeCommand + " "); p.caretToEnd = true }}}
	for i, m := range p.modes {
		all = append(all, item{key: "/" + m.ID, title: m.Name, desc: clip(m.Desc),
			take: func(p *Panel, set func(string)) { p.setMode(i); set("") }})
	}
	all = append(all, item{key: "/note", title: "笔记",
		take: func(p *Panel, set func(string)) { set(""); p.setMode(len(p.modes)) }})
	return filter(all, "/"+query)
}

// topics are what operator mode is most often asked, each a question it
// answers or, for themes, the picker.
var topics = []struct{ key, title, ask string }{
	{"theme", "切换配色主题", ""},
	{"自定义主题", "改颜色，或基于内置主题新建一套", "基于当前主题新建一套自定义主题之前，先告诉我能改哪些颜色，以及现在每个颜色的值。"},
	{"模型", "各模式用哪个模型，默认模型是哪个", "现在各模式用的是哪个模型？默认模型是哪个？"},
	{"快捷键", "查看或修改全局快捷键", "现在的快捷键有哪些？"},
	{"模式", "查看模式和完整提示词，增删模式", "列出所有模式，以及每个模式的提示词。"},
	{"权限", "哪些系统权限还没给", "哪些权限还没给？分别影响什么？"},
	{"语言", "我的语言和外语", "我的语言和外语分别是什么？"},
	{"开机启动", "登录时自动打开", "登录时自动打开现在是开着还是关着？"},
	{"配置模型", "交给 Coding Agent 配置模型服务", "我想配置或更换模型服务。"},
}

func (p *Panel) topicItems(query string) []item {
	var all []item
	for _, t := range topics {
		all = append(all, item{key: t.key, title: t.title, take: func(p *Panel, set func(string)) {
			if t.ask == "" {
				set(themeCommand + " ")
				p.caretToEnd = true
				return
			}
			set(operator.Command + " " + t.ask)
			p.source = operator.Command + " " + t.ask
			p.caretToEnd = true
			p.run()
		}})
	}
	return filter(all, query)
}

func (p *Panel) themeItems(query string) []item {
	all := []item{{key: "auto", title: "跟随系统", desc: design.AutoDark + " / " + design.AutoLight, take: takeTheme("auto")}}
	var custom []config.Theme
	if p.cfg != nil {
		custom = p.cfg.Themes
	}
	for _, theme := range custom {
		title := theme.Name
		if title == "" {
			title = theme.ID
		}
		t := p.custom[theme.ID]
		all = append(all, item{key: theme.ID, title: title, desc: "基于 " + theme.Base,
			swatch: []ui.Color{t.Background, t.Accent, t.Code, t.Quote, t.Sparkle}, take: takeTheme(theme.ID)})
	}
	for _, name := range design.ThemeNames {
		t := design.Themes[name]
		mode := "浅色"
		if t.Dark {
			mode = "深色"
		}
		all = append(all, item{key: name, title: mode, swatch: []ui.Color{t.Background, t.Accent, t.Code, t.Quote, t.Sparkle}, take: takeTheme(name)})
	}
	return filter(all, query)
}

// takeTheme keeps the theme previewed, writing it to config.json.
func takeTheme(name string) func(p *Panel, set func(string)) {
	return func(p *Panel, set func(string)) {
		p.setConfig("theme", name, func(c *config.Config) { c.Theme = name })
		set("")
	}
}

// filter keeps the items whose key or title has query, those starting with
// it first.
func filter(all []item, query string) []item {
	q := strings.ToLower(query)
	var first, rest []item
	for _, it := range all {
		key := strings.ToLower(it.key)
		switch {
		case strings.HasPrefix(key, q) || strings.HasPrefix(it.title, query):
			first = append(first, it)
		case strings.Contains(key, q) || strings.Contains(it.title, query):
			rest = append(rest, it)
		}
	}
	return append(first, rest...)
}

func clip(s string) string {
	if r := []rune(s); len(r) > 48 {
		return string(r[:48]) + "…"
	}
	return s
}

// menuWaiting reports whether a menu that just opened or grew waits for the
// window: until a frame has measured the panel with it, and the window is
// as tall as that.
func (p *Panel) menuWaiting(c *ui.Context, rows int) bool {
	if rows > p.menuRows {
		p.menuGrew = 0
	}
	p.menuRows = rows
	if p.menuGrew < 1 {
		// This frame lays the menu out and measures it as it is drawn.
		p.menuGrew++
		c.Invalidate()
		return true
	}
	_, windowH := c.Size()
	if windowH+1 < p.panelH && p.menuGrew < menuWaitFrames {
		// A window that does not grow, as at the screen's limit, is not
		// waited for long.
		p.menuGrew++
		c.Invalidate()
		return true
	}
	return false
}

// menuWaitFrames bounds how long a menu waits for the window.
const menuWaitFrames = 10

// menuKey handles a key while a menu shows.
func (p *Panel) menuKey(k ui.TextKey, pal *design.Palette) bool {
	kind, items := p.menu(pal)
	if kind == menuNone {
		return false
	}
	switch {
	case k.Key == ui.KeyEscape && k.Mods == 0:
		p.menuClosed = p.source
		p.menuChosenFor = ""
		return true
	case len(items) == 0:
		return false
	case k.Key == ui.KeyDown && k.Mods == 0:
		p.menuIdx = (p.menuIdx + 1) % len(items)
		p.menuChosenFor = p.source
		return true
	case k.Key == ui.KeyUp && k.Mods == 0:
		p.menuIdx = (p.menuIdx - 1 + len(items)) % len(items)
		p.menuChosenFor = p.source
		return true
	case k.Key == ui.KeyEnter && k.Mods == 0 && p.source == operator.Command && p.menuChosenFor != p.source:
		p.menuClosed = p.source
		return false
	case (k.Key == ui.KeyTab || k.Key == ui.KeyEnter) && k.Mods == 0:
		it := items[min(p.menuIdx, len(items)-1)]
		p.menuIdx = 0
		it.take(p, k.SetText)
		return true
	}
	return false
}

// previewTheme is the theme chosen in the theme picker, shown until the
// picker closes; "" when no picker shows.
func (p *Panel) previewTheme(pal *design.Palette) string {
	kind, items := p.menu(pal)
	if kind != menuThemes || len(items) == 0 {
		return ""
	}
	return items[min(p.menuIdx, len(items)-1)].key
}

// menuView lists the menu's items under the source.
func (p *Panel) menuView(c *ui.Context, pal *design.Palette) {
	kind, items := p.menu(pal)
	if kind == menuNone {
		p.menuRows = 0
		return
	}
	ui.Divider(c).Background(pal.Border)
	menu := ui.Column(c.Key("menu")).Shrink(0).Padding(6, 10).Gap(1)
	p.menuHidden = p.menuWaiting(c, len(items))
	if p.menuHidden {
		// Laid out but not drawn until the window has grown to hold it:
		// drawn before, the window's edge would cut it, and it would show
		// a row at a time as the window catches up.
		menu.Invisible()
	}
	menu.Children(func() {
		if len(items) == 0 {
			ui.Text(c, "没有匹配的项").FontSize(p.sz.Label).TextColor(pal.Faint).Padding(6, 10)
			return
		}
		sel := min(p.menuIdx, len(items)-1)
		list := ui.Scroll(c.Key("items")).MaxHeight(260).Gap(1)
		list.Children(func() {
			for i, it := range items {
				row := ui.Row(c.Key(it.key)).Gap(10).Padding(6, 10).Radius(design.RadiusChip)
				if i == sel {
					row.Background(pal.Raised)
					row.ScrollIntoView()
				}
				row.Children(func() {
					color := it.color
					if color.A == 0 {
						color = pal.Bright
					}
					ui.Text(c, it.key).FontSize(p.sz.Label).FontWeight(600).TextColor(color).Width(150).MaxLines(1)
					if len(it.swatch) > 0 {
						ui.Row(c).Gap(3).Children(func() {
							for _, col := range it.swatch {
								ui.Box(c).Size(12, 12).Radius(2).Background(col).Border(1, pal.Border)
							}
						})
					}
					ui.Text(c, it.title).FontSize(p.sz.Label).TextColor(pal.Text).MinWidth(70)
					ui.Text(c, it.desc).FontSize(p.sz.Caption).TextColor(pal.Muted).Grow(1).MaxLines(1)
				})
				if row.Clicked() {
					p.menuIdx = 0
					it.take(p, func(s string) { p.source = s })
					if !p.noteMode() {
						p.sourceIn.Focus()
					}
				}
			}
		})
	})
}
