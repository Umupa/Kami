package panel

import (
	"strings"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/cida-next/internal/design"
	"github.com/Umupa/cida-next/internal/operator"
)

// command is an entry of the slash menu: operator mode, or a mode to switch
// to.
type command struct {
	name  string // what follows the slash
	title string
	desc  string
	mode  int // the mode it switches to, -1 for operator
}

// commands lists what the slash menu offers, filtered by what follows the
// slash: matching the name or the title, names first.
func (p *Panel) commands(query string) []command {
	all := []command{{name: "operator", title: "Operator", desc: "用一句话查看或修改 cida-next 的设置", mode: -1}}
	for i, m := range p.modes {
		all = append(all, command{name: m.ID, title: m.Name, desc: clip(m.Desc), mode: i})
	}
	q := strings.ToLower(query)
	var first, rest []command
	for _, c := range all {
		switch {
		case strings.HasPrefix(c.name, q) || strings.HasPrefix(c.title, query):
			first = append(first, c)
		case strings.Contains(c.name, q) || strings.Contains(c.title, query):
			rest = append(rest, c)
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

// slashQuery returns what follows the slash while the source is a command
// being typed: a slash and a word, nothing after it yet.
func (p *Panel) slashQuery() (string, bool) {
	if p.slashClosed == p.source || !strings.HasPrefix(p.source, "/") || strings.ContainsAny(p.source, " \t\n") {
		return "", false
	}
	if _, isOperator := operator.Leading(p.source); isOperator {
		return "", false
	}
	return p.source[1:], true
}

// slashKey handles a key while the menu shows: the arrows move the choice,
// Tab and Enter take it, Escape closes the menu.
func (p *Panel) slashKey(k ui.TextKey) bool {
	query, ok := p.slashQuery()
	if !ok {
		return false
	}
	cmds := p.commands(query)
	switch {
	case k.Key == ui.KeyEscape && k.Mods == 0:
		p.slashClosed = p.source
		return true
	case len(cmds) == 0:
		return false
	case k.Key == ui.KeyDown && k.Mods == 0:
		p.slashIdx = (p.slashIdx + 1) % len(cmds)
		return true
	case k.Key == ui.KeyUp && k.Mods == 0:
		p.slashIdx = (p.slashIdx - 1 + len(cmds)) % len(cmds)
		return true
	case (k.Key == ui.KeyTab || k.Key == ui.KeyEnter) && k.Mods == 0:
		p.pick(cmds[min(p.slashIdx, len(cmds)-1)], k.SetText)
		return true
	}
	return false
}

// pick takes a command: operator mode keeps "/operator " in the source for
// the request to follow; a mode is switched to and its command removed.
func (p *Panel) pick(cmd command, setText func(string)) {
	p.slashIdx = 0
	if cmd.mode < 0 {
		setText(operator.Command + " ")
		p.caretToEnd = true
		return
	}
	p.modeIdx = cmd.mode
	setText("")
}

// slashMenu lists the commands under the source while one is typed, as
// monocode's quick composer does.
func (p *Panel) slashMenu(c *ui.Context, pal *design.Palette) {
	query, ok := p.slashQuery()
	if !ok {
		return
	}
	cmds := p.commands(query)
	ui.Divider(c).Background(pal.Border)
	ui.Column(c.Key("slash")).Shrink(0).Padding(6, 10).Gap(1).Children(func() {
		if len(cmds) == 0 {
			ui.Text(c, "没有匹配的命令").FontSize(design.FontLabel).TextColor(pal.Faint).Padding(6, 10)
			return
		}
		sel := min(p.slashIdx, len(cmds)-1)
		for i, cmd := range cmds {
			row := ui.Row(c.Key(cmd.name)).Gap(10).Padding(6, 10).Radius(design.RadiusChip)
			if i == sel {
				row.Background(pal.Raised)
			}
			row.Children(func() {
				color := pal.Bright
				if cmd.mode < 0 {
					color = pal.Operator
				}
				ui.Text(c, "/"+cmd.name).FontSize(design.FontLabel).FontWeight(600).TextColor(color).Width(110)
				ui.Text(c, cmd.title).FontSize(design.FontLabel).TextColor(pal.Text).Width(90)
				ui.Text(c, cmd.desc).FontSize(design.FontCaption).TextColor(pal.Muted).Grow(1).MaxLines(1)
			})
			if row.Clicked() {
				p.pick(cmd, func(s string) { p.source = s })
				p.sourceIn.Focus()
			}
		}
		ui.Row(c).Gap(14).Padding(6, 10, 2, 10).Children(func() {
			for _, hint := range []string{"↑↓ 选择", "tab ⏎ 确定", "esc 关闭"} {
				ui.Text(c, hint).FontSize(design.FontCaption).TextColor(pal.Faint)
			}
		})
	})
}
