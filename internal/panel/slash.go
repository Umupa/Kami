package panel

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/design"
	"github.com/Umupa/cida-next/internal/operator"
)

// A menu shows under the source while a command is typed, as monocode's
// quick composer does: the commands after "/", the topics after
// "/operator ", the themes after "/operator theme". The arrows choose, Tab
// and Enter take, Escape closes.
type menuKind int

const (
	menuNone menuKind = iota
	menuCommands
	menuTopics
	menuThemes
	menuFonts
)

// item is an entry of a menu.
type item struct {
	key, title, desc string
	color            ui.Color   // the key's, zero for the default
	swatch           []ui.Color // a theme's colors
	font             string     // a font preset's family, to show its name in it
	take             func(p *Panel, setText func(string))
}

// themeCommand and fontCommand open the pickers.
const (
	themeCommand = operator.Command + " theme"
	fontCommand  = operator.Command + " font"
)

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
	if src == p.menuClosed || strings.Contains(src, "\n") {
		return menuNone, nil
	}
	switch {
	case strings.HasPrefix(src, fontCommand) && (len(src) == len(fontCommand) || src[len(fontCommand)] == ' '):
		return menuFonts, p.fontItems(strings.TrimSpace(src[len(fontCommand):]))
	case strings.HasPrefix(src, themeCommand) && (len(src) == len(themeCommand) || src[len(themeCommand)] == ' '):
		return menuThemes, p.themeItems(strings.TrimSpace(src[len(themeCommand):]))
	case strings.HasPrefix(src, operator.Command+" "):
		rest := src[len(operator.Command)+1:]
		if strings.Contains(rest, " ") {
			return menuNone, nil
		}
		return menuTopics, p.topicItems(rest)
	case strings.HasPrefix(src, "/") && !strings.ContainsAny(src, " \t"):
		if _, ok := operator.Leading(src); ok {
			return menuNone, nil
		}
		return menuCommands, p.commandItems(src[1:], pal)
	}
	return menuNone, nil
}

func (p *Panel) commandItems(query string, pal *design.Palette) []item {
	all := []item{{key: "/operator", title: "Operator", desc: "用一句话查看或修改 cida-next 的设置", color: pal.Operator,
		take: func(p *Panel, set func(string)) { set(operator.Command + " "); p.caretToEnd = true }}}
	for i, m := range p.modes {
		all = append(all, item{key: "/" + m.ID, title: m.Name, desc: clip(m.Desc),
			take: func(p *Panel, set func(string)) { p.modeIdx = i; set("") }})
	}
	return filter(all, "/"+query)
}

// topics are what operator mode is most often asked, each a question it
// answers or, for themes, the picker.
var topics = []struct{ key, title, ask string }{
	{"theme", "切换配色主题", ""},
	{"font", "切换字体，⌘= ⌘- 调字号", ""},
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
				set(operator.Command + " " + t.key + " ")
				p.caretToEnd = true
				return
			}
			set(operator.Command + " " + t.ask)
			p.source = operator.Command + " " + t.ask
			p.run()
		}})
	}
	return filter(all, query)
}

func (p *Panel) themeItems(query string) []item {
	all := []item{{key: "auto", title: "跟随系统", desc: design.AutoDark + " / " + design.AutoLight, take: takeTheme("auto")}}
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
		p.notice = "主题已切换为 " + name + "，可以随时用 /operator theme 再换。"
		set("")
	}
}

func (p *Panel) fontItems(query string) []item {
	var all []item
	for _, f := range design.Fonts {
		label, sample := f.Label, "敏捷的狐狸 The quick fox 0123"
		all = append(all, item{key: f.Name, title: label, desc: sample, font: f.Use().Family, take: func(p *Panel, set func(string)) {
			p.setConfig("font", f.Name, func(c *config.Config) { c.Font = f.Name })
			p.notice = "字体已切换为 " + label + "；⌘= ⌘- 调字号，⌘0 恢复。"
			set("")
		}})
	}
	return filter(all, query)
}

// previewFont is the font chosen in the font picker, shown until the picker
// closes; "" when no picker shows.
func (p *Panel) previewFont() string {
	kind, items := p.menu(p.pal)
	if kind != menuFonts || len(items) == 0 {
		return ""
	}
	return items[min(p.menuIdx, len(items)-1)].key
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

// menuKey handles a key while a menu shows.
func (p *Panel) menuKey(k ui.TextKey, pal *design.Palette) bool {
	kind, items := p.menu(pal)
	if kind == menuNone {
		return false
	}
	switch {
	case k.Key == ui.KeyEscape && k.Mods == 0:
		p.menuClosed = p.source
		return true
	case len(items) == 0:
		return false
	case k.Key == ui.KeyDown && k.Mods == 0:
		p.menuIdx = (p.menuIdx + 1) % len(items)
		return true
	case k.Key == ui.KeyUp && k.Mods == 0:
		p.menuIdx = (p.menuIdx - 1 + len(items)) % len(items)
		return true
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
		return
	}
	ui.Divider(c).Background(pal.Border)
	ui.Column(c.Key("menu")).Shrink(0).Padding(6, 10).Gap(1).Children(func() {
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
					title := ui.Text(c, it.title).FontSize(p.sz.Label).TextColor(pal.Text).MinWidth(70)
					desc := ui.Text(c, it.desc).FontSize(p.sz.Caption).TextColor(pal.Muted).Grow(1).MaxLines(1)
					if kind == menuFonts {
						// Each font shows its name and a sample in itself.
						title.Font(it.font)
						desc.Font(it.font).FontSize(p.sz.Label)
					}
				})
				if row.Clicked() {
					p.menuIdx = 0
					it.take(p, func(s string) { p.source = s })
					p.sourceIn.Focus()
				}
			}
		})
		hint := "tab ⏎ 确定"
		switch kind {
		case menuThemes:
			hint = "⏎ 用这个主题"
		case menuFonts:
			hint = "⏎ 用这个字体"
		}
		ui.Row(c).Gap(14).Padding(6, 10, 2, 10).Children(func() {
			for _, h := range []string{"↑↓ 选择", hint, "esc 关闭"} {
				ui.Text(c, h).FontSize(p.sz.Caption).TextColor(pal.Faint)
			}
		})
	})
}
