package settings

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/design"
	"github.com/Umupa/cida-next/internal/mode"
)

func (s *Settings) view(c *ui.Context) {
	pal := design.For(c.Theme().Dark)
	c.Root().Background(pal.Background)
	ui.Column(c).Fill().Children(func() {
		ui.Row(c).Padding(16, 24, 0, 24).Children(func() {
			if ui.Tabs(c, &s.tab, "模型", "模式", "快捷键与权限", "通用").Changed() {
				s.stopRecording()
			}
		})
		ui.Scroll(c).Grow(1).Padding(20, 24).Gap(14).Children(func() {
			if s.snap.Err != nil {
				s.card(c, pal, func() {
					ui.Text(c, "config.json 有问题，应用在用上一份合法配置").FontSize(design.FontSecondary).FontWeight(600).TextColor(pal.Text)
					ui.Text(c, s.snap.Err.Error()).FontSize(design.FontLabel).TextColor(pal.TextMuted).Selectable()
					s.agentRow(c, pal)
				})
			}
			if s.editErr != "" {
				ui.Text(c, "没有保存："+s.editErr).FontSize(design.FontLabel).TextColor(c.Theme().Danger)
			}
			switch s.tab {
			case 0:
				s.modelsTab(c, pal)
			case 1:
				s.modesTab(c, pal)
			case 2:
				s.shortcutsTab(c, pal)
			case 3:
				s.generalTab(c, pal)
			}
		})
	})
}

func (s *Settings) card(c *ui.Context, pal *design.Palette, children func()) {
	ui.Column(c).Gap(8).Padding(16, 18).Radius(design.RadiusCard).Border(1, pal.Border).
		Background(pal.SurfacePaper).Children(children)
}

// agentRow offers the configuration prompt, the way most settings change.
func (s *Settings) agentRow(c *ui.Context, pal *design.Palette) {
	ui.Row(c).Gap(10).Children(func() {
		ui.Text(c, "调整配置 · 交给 Claude Code 等 AI 助手").FontSize(design.FontLabel).TextColor(pal.TextMuted).Grow(1)
		label := "复制配置提示词"
		if time.Since(s.copied) < design.MotionCopiedHold {
			label = "✓ 已复制"
			c.After(design.MotionCopiedHold)
		}
		if ui.Button(c, label).Clicked() {
			s.deps.CopyPrompt()
			s.copied = time.Now()
		}
	})
}

func (s *Settings) modelsTab(c *ui.Context, pal *design.Palette) {
	cfg := s.snap.Config
	if len(cfg.Models) == 0 {
		s.card(c, pal, func() {
			ui.Text(c, "还没有模型服务").FontSize(design.FontBody).FontWeight(600).TextColor(pal.Text)
			ui.Text(c, "复制配置提示词，交给 Claude Code、Codex 等 AI 助手。它会问你用哪家服务，配好后自己检查。").
				FontSize(design.FontSecondary).TextColor(pal.TextMuted)
			s.agentRow(c, pal)
		})
		return
	}
	for _, m := range cfg.Models {
		s.card(c.Key(m.Name), pal, func() {
			ui.Row(c).Gap(10).Children(func() {
				title := m.Name
				if m.Name == cfg.DefaultModel {
					title += "（默认）"
				}
				ui.Text(c, title).FontSize(design.FontBody).FontWeight(600).TextColor(pal.Text).Grow(1)
				ck := s.checks[m.Name]
				if ui.Button(c, map[bool]string{true: "检查中…", false: "检查"}[ck.running]).Disabled(ck.running).Clicked() {
					s.runCheck(m)
				}
			})
			ui.Text(c, fmt.Sprintf("%s · %s · %s", m.Model, m.Format, host(m.Endpoint))).
				FontSize(design.FontSecondary).TextColor(pal.TextMuted)
			ui.Text(c, keyState(m)).FontSize(design.FontLabel).TextColor(pal.TextMuted)
			if ck := s.checks[m.Name]; !ck.at.IsZero() {
				mark, color := "✗ ", pal.Text
				if ck.ok {
					mark, color = "✓ ", pal.Accent
				}
				line := mark + ck.message
				if ck.took > 0 {
					line += fmt.Sprintf(" · %d ms", ck.took.Milliseconds())
				}
				ui.Text(c, line).FontSize(design.FontLabel).TextColor(color)
				if !ck.ok {
					ui.Text(c, "复制配置提示词，交给 AI 助手继续检查。").FontSize(design.FontLabel).TextColor(pal.TextMuted)
				}
			}
		})
	}
	s.agentRow(c, pal)
}

func keyState(m config.Model) string {
	switch {
	case m.Auth == config.AuthNone:
		return "不需要 API Key"
	case m.APIKey != "":
		return "API Key 已配置"
	}
	return "还差 API Key：在 config.json 里给这个模型加上 apiKey，或复制配置提示词交给 AI 助手。"
}

func host(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		return u.Host
	}
	return endpoint
}

func (s *Settings) modesTab(c *ui.Context, pal *design.Palette) {
	cfg := s.snap.Config
	s.card(c, pal, func() {
		row(c, pal, "我的语言", cfg.MyLanguage)
		row(c, pal, "外语", cfg.ForeignLanguage)
	})
	modes, err := mode.Resolve(cfg)
	if err != nil {
		return
	}
	for i, m := range modes {
		s.card(c.Key(m.ID), pal, func() {
			ui.Row(c).Gap(8).Children(func() {
				name := m.Name
				if i == 0 {
					name += "（默认）"
				}
				ui.Text(c, name).FontSize(design.FontBody).FontWeight(600).TextColor(pal.Text).Grow(1)
				model := m.Model
				if model == "" {
					model = "未配置模型"
				}
				ui.Text(c, model).FontSize(design.FontLabel).TextColor(pal.TextMuted)
			})
			ui.Text(c, firstLine(m.Prompt)).FontSize(design.FontLabel).TextColor(pal.TextMuted).MaxLines(2)
		})
	}
	ui.Text(c, "模式在 config.json 的 modes 里增删、排序、改提示词：第一项是默认模式。让 AI 助手按 schema 修改最方便。").
		FontSize(design.FontLabel).TextColor(pal.TextMuted)
	s.agentRow(c, pal)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

func row(c *ui.Context, pal *design.Palette, label, value string) {
	ui.Row(c).Gap(12).Children(func() {
		ui.Text(c, label).FontSize(design.FontSecondary).TextColor(pal.TextMuted).Width(96)
		ui.Text(c, value).FontSize(design.FontSecondary).TextColor(pal.Text).Grow(1)
	})
}

func (s *Settings) shortcutsTab(c *ui.Context, pal *design.Palette) {
	sc := s.snap.Config.Shortcuts
	s.card(c, pal, func() {
		s.shortcutRow(c, pal, "panel", "打开面板", sc.Panel, true)
		s.shortcutRow(c, pal, "selection", "带入选中的文字", sc.Selection, false)
		s.shortcutRow(c, pal, "capture", "截图识别", sc.Capture, false)
	})
	if s.recording != "" {
		ui.Text(c, "按下新的快捷键，至少带 ⌃、⌥ 或 ⌘ 之一；Esc 取消。").FontSize(design.FontLabel).TextColor(pal.Accent)
	}
	s.card(c, pal, func() {
		for _, p := range s.deps.Permissions {
			ui.Row(c.Key(p.Name)).Gap(10).Children(func() {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, p.Name).FontSize(design.FontSecondary).TextColor(pal.Text)
					ui.Text(c, p.Purpose).FontSize(design.FontLabel).TextColor(pal.TextMuted)
				})
				if p.Granted() {
					ui.Text(c, "✓ 已允许").FontSize(design.FontLabel).TextColor(pal.Accent)
				} else if ui.Button(c, "去允许").Clicked() {
					p.Request()
				}
			})
		}
		ui.Text(c, "允许后，退出并重新打开 cida-next 才会生效。").FontSize(design.FontLabel).TextColor(pal.TextMuted)
	})
	// Permissions change in System Settings; look again while the tab shows.
	c.After(2 * time.Second)
}

func (s *Settings) shortcutRow(c *ui.Context, pal *design.Palette, field, label, value string, doubleTaps bool) {
	ui.Row(c.Key(field)).Gap(10).Children(func() {
		ui.Text(c, label).FontSize(design.FontSecondary).TextColor(pal.Text).Grow(1)
		if s.recording == field {
			s.recorder(c, pal)
		} else {
			ui.Text(c, Glyphs(value)).FontSize(design.FontSecondary).TextColor(pal.TextControl)
		}
		ui.Button(c, "更改").Menu(func(m *ui.Menu) {
			if m.Item("录制组合键").Chosen() {
				s.startRecording(field)
			}
			if doubleTaps {
				for _, mod := range []string{"option", "control", "command"} {
					v := "double-" + mod
					if m.Item(Glyphs(v)).Checked(value == v).Chosen() {
						s.setShortcut(field, v)
					}
				}
			}
			if m.Item("不设置").Checked(value == "none").Chosen() {
				s.setShortcut(field, "none")
			}
		})
	})
}

func (s *Settings) setShortcut(field, value string) {
	s.edit(func(c *config.Config) {
		switch field {
		case "panel":
			c.Shortcuts.Panel = value
		case "selection":
			c.Shortcuts.Selection = value
		case "capture":
			c.Shortcuts.Capture = value
		}
	})
}

// recorder holds the focus while a shortcut is recorded and takes the next
// key combination pressed.
func (s *Settings) recorder(c *ui.Context, pal *design.Palette) {
	ui.Box(c.Key("recorder")).Focusable().Focus().PaddingX(8).PaddingY(2).Radius(design.RadiusChip).
		Border(1, pal.Accent).Children(func() {
		ui.Text(c, "按下快捷键…").FontSize(design.FontSecondary).TextColor(pal.Accent)
	}).HandleInput(func(ev ui.InputEvent) bool {
		if ev.Kind != ui.InputKeyDown {
			return false
		}
		if ev.Key == ui.KeyEscape && ev.Mods == 0 {
			s.stopRecording()
			return true
		}
		v, ok := combination(ev.Key, ev.Mods)
		if !ok {
			return true // a modifier alone, or a key shortcuts cannot use
		}
		field := s.recording
		s.stopRecording()
		s.setShortcut(field, v)
		return true
	})
}

// combination spells a pressed key in config.json's shortcut syntax.
func combination(key ui.Key, mods ui.Modifiers) (string, bool) {
	name, ok := keyNames[key]
	if !ok {
		return "", false
	}
	var parts []string
	for _, m := range []struct {
		mod  ui.Modifiers
		name string
	}{{ui.Ctrl, "control"}, {ui.Alt, "option"}, {ui.Shift, "shift"}, {ui.Super, "command"}} {
		if mods&m.mod != 0 {
			parts = append(parts, m.name)
		}
	}
	v := strings.Join(append(parts, name), "+")
	if _, err := config.ParseShortcut(v); err != nil {
		return "", false
	}
	return v, true
}

var keyNames = func() map[ui.Key]string {
	m := map[ui.Key]string{ui.KeySpace: "space"}
	for i := range 26 {
		m[ui.KeyA+ui.Key(i)] = string(rune('a' + i))
	}
	for i := range 10 {
		m[ui.Key0+ui.Key(i)] = string(rune('0' + i))
	}
	for i, k := range []ui.Key{ui.KeyF1, ui.KeyF2, ui.KeyF3, ui.KeyF4, ui.KeyF5, ui.KeyF6, ui.KeyF7, ui.KeyF8, ui.KeyF9, ui.KeyF10, ui.KeyF11, ui.KeyF12} {
		m[k] = fmt.Sprintf("f%d", i+1)
	}
	return m
}()

// Glyphs shows a shortcut as macOS menus do: ⌃⌥A, 双击 ⌥, 未设置.
func Glyphs(value string) string {
	sc, err := config.ParseShortcut(value)
	if err != nil {
		return value
	}
	glyph := map[config.Modifiers]string{config.ModControl: "⌃", config.ModOption: "⌥", config.ModShift: "⇧", config.ModCommand: "⌘"}
	switch {
	case sc.IsZero():
		return "未设置"
	case sc.Double != 0:
		return "双击 " + glyph[sc.Double]
	}
	var b strings.Builder
	for _, m := range []config.Modifiers{config.ModControl, config.ModOption, config.ModShift, config.ModCommand} {
		if sc.Mods&m != 0 {
			b.WriteString(glyph[m])
		}
	}
	b.WriteString(strings.ToUpper(sc.Key))
	return b.String()
}

func (s *Settings) generalTab(c *ui.Context, pal *design.Palette) {
	cfg := s.snap.Config
	s.card(c, pal, func() {
		ui.Row(c).Gap(10).Children(func() {
			ui.Text(c, "登录时打开").FontSize(design.FontSecondary).TextColor(pal.Text).Grow(1)
			on := cfg.LaunchAtLogin
			if ui.Switch(c, &on).Label("登录时打开").Changed() {
				s.edit(func(c *config.Config) { c.LaunchAtLogin = on })
			}
		})
	})
	s.card(c, pal, func() {
		row(c, pal, "版本", s.version)
		row(c, pal, "配置文件", s.deps.Store.Path())
		ui.Row(c).Children(func() {
			ui.Spacer(c)
			if ui.Button(c, "在访达中显示").Clicked() {
				s.deps.RevealConfig()
			}
		})
	})
}
