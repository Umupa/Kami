package panel

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/design"
)

const designReply = "## 会议纪要\n\n今天讨论了**三件事**，配置文件在 `~/Library/Application Support/Kami/config.json`，详见[文档](https://example.com)。\n\n" +
	"| 事项 | 负责人 | 截止 |\n|---|---|---|\n| 面板改版 | 我 | 周五 |\n| Operator | Claude | 已完成 |\n\n" +
	"1. 先读配置\n2. 再改快捷键\n   - 截图改成 `option+s`\n\n" +
	"```go\nfunc main() {\n\t// 打个招呼\n\tfmt.Println(\"hello\", 42)\n}\n```\n\n" +
	"> 引用：Key 不会出现在任何输出里。\n"

func renderDesign(t *testing.T, name string, dark bool) {
	h := newHarness(t, script{pieces: []string{designReply}})
	h.p.place = placeOn(mygo.Rectangle{Width: 1440, Height: 1500})
	h.tt.SetSize(820, 1100)
	h.tt.SetDark(dark)
	h.p.cfg.Theme = "auto" // the pair that follows the appearance
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("Summarize today's meeting")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Click("追问")
	h.tt.Type("再短一点")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.snapshot(name)
}

func TestDesignSnapshots(t *testing.T) {
	renderDesign(t, "design-dark", true)
	renderDesign(t, "design-light", false)

	h := newHarness(t, script{})
	h.tt.SetDark(true)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("/")
	h.tt.Frame()
	if !h.tt.HasText("/operator") {
		t.Errorf("typing / lists no commands: %q", h.tt.Texts())
	}
	h.snapshot("slash-menu")
	h.tt.Key(0, ui.KeyDown)
	h.tt.Key(0, ui.KeyDown)
	h.tt.Key(0, ui.KeyTab)
	h.tt.Frame()
	if h.p.source != "" || h.p.modes[h.p.modeIdx].ID != "translate" {
		t.Errorf("picking the first mode, after /operator and /theme: source %q, mode %s", h.p.source, h.p.modes[h.p.modeIdx].ID)
	}
	h.tt.Type("/op")
	h.tt.Key(0, ui.KeyEnter)
	if h.p.source != "/operator " {
		t.Errorf("Enter on /op gave %q, want /operator", h.p.source)
	}
}

func TestThemePicker(t *testing.T) {
	var applied []string
	h := newHarness(t, script{})
	h.p.deps.Operator.Apply = func(ops []config.Op) ([]config.Change, error) {
		applied = append(applied, string(ops[0].Value))
		return nil, nil
	}
	h.tt.SetDark(true)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("/operator ")
	h.tt.Frame()
	if !h.tt.HasText("切换配色主题") || !h.tt.HasText("快捷键") {
		t.Fatalf("no topics after /operator: %q", h.tt.Texts())
	}
	h.snapshot("operator-topics")
	h.tt.Key(0, ui.KeyEnter) // theme, the first topic
	h.tt.Frame()
	if h.p.source != "/theme " {
		t.Fatalf("source %q", h.p.source)
	}
	h.tt.Type("gruv")
	h.tt.Frame()
	if got := h.p.previewTheme(h.p.pal); got != "gruvbox" {
		t.Errorf("previewing %q, want gruvbox", got)
	}
	h.snapshot("theme-picker")
	h.tt.Key(0, ui.KeyEnter)
	h.tt.Frame()
	if len(applied) != 1 || applied[0] != `"gruvbox"` || h.p.cfg.Theme != "gruvbox" {
		t.Errorf("applied %v, theme %q", applied, h.p.cfg.Theme)
	}
	h.snapshot("theme-gruvbox")
}

func TestFontSize(t *testing.T) {
	var applied []config.Op
	h := newHarness(t, script{pieces: []string{designReply}})
	h.p.place = placeOn(mygo.Rectangle{Width: 1440, Height: 1500})
	h.tt.SetSize(820, 1100)
	h.p.deps.Operator.Apply = func(ops []config.Op) ([]config.Change, error) {
		applied = append(applied, ops...)
		return nil, nil
	}
	h.tt.SetDark(true)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Key(ui.Cmd, ui.KeyEqual)
	h.tt.Key(ui.Cmd, ui.KeyEqual)
	h.tt.Frame()
	if h.p.sz.Body != 15.5 {
		t.Errorf("after two ⌘= the body is %v", h.p.sz.Body)
	}
	h.tt.Click("原文")
	h.tt.Type("hello")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.snapshot("font-caskaydia")
	h.tt.Key(ui.Cmd, ui.Key0)
	h.tt.Frame()
	if h.p.sz.Body != 13.5 {
		t.Errorf("⌘0 left the body at %v", h.p.sz.Body)
	}
	if len(applied) != 3 {
		t.Errorf("applied %d edits, want three size changes", len(applied))
	}
}

func TestPinAndUnmatchedMenu(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Key(ui.Cmd|ui.Shift, ui.KeyP)
	if !h.p.pinned {
		t.Error("⌘⇧P did not pin the panel")
	}
	h.tt.Type("/operator 我的语言和外语分别是什么？")
	h.tt.Frame()
	if kind, _ := h.p.menu(h.p.pal); kind != menuNone || h.tt.HasText("没有匹配的项") {
		t.Error("a request that matches no topic still shows a menu")
	}
	h.p.source = "/zzz"
	h.tt.Frame()
	if kind, _ := h.p.menu(h.p.pal); kind != menuNone {
		t.Error("an unknown command shows an empty menu")
	}
}

func TestThemeCommand(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("/theme kanagawa")
	h.tt.Frame()
	if got := h.p.previewTheme(h.p.pal); got != "kanagawa" {
		t.Errorf("previewing %q, want kanagawa", got)
	}
	h.tt.Type("zzz")
	h.tt.Frame()
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	if h.p.sess != nil {
		t.Error("Enter on a theme no name matches sent it to the model")
	}
}

func TestCustomThemeRenderingAndPicker(t *testing.T) {
	h := newHarness(t, script{pieces: []string{designReply}})
	h.p.place = placeOn(mygo.Rectangle{Width: 1440, Height: 1500})
	h.tt.SetSize(820, 1100)
	c := *h.p.cfg
	c.Theme = "my-night"
	c.Themes = []config.Theme{{ID: "my-night", Name: "我的夜间", Base: "flexoki-night", Colors: map[string]string{"accent": "#DA702C"}}, {ID: "plain-name", Base: "gruvbox"}}
	h.p.applyConfig(&c)
	h.tt.Frame()
	pal := h.p.pal
	if pal.Accent != ui.Hex("#DA702C") || pal == design.Themes["flexoki-night"] {
		t.Fatal("custom palette not rendered")
	}
	for range 3 {
		h.tt.Frame()
		if h.p.pal != pal || h.p.mdFor != (mdKey{pal, h.p.sz.Body}) {
			t.Fatal("palette pointer changed between frames")
		}
	}
	h.p.source = "Summarize today's meeting"
	h.p.run()
	h.drain()
	h.snapshot("theme-custom")
	items := h.p.themeItems("")
	if items[0].key != "auto" || items[1].key != "my-night" || items[1].title != "我的夜间" || items[1].desc != "基于 flexoki-night" || items[2].title != "plain-name" {
		t.Fatalf("custom menu: %+v", items[:3])
	}
	h.p.cfg.Theme = "gruvbox"
	h.p.source = "/theme my-night"
	h.tt.Frame()
	if h.p.previewTheme(h.p.pal) != "my-night" || h.p.pal != pal {
		t.Fatal("custom preview missing")
	}
	h.p.menuClosed = h.p.source
	h.tt.Frame()
	if h.p.pal != design.Themes["gruvbox"] {
		t.Fatal("closing preview did not restore theme")
	}
	var applied string
	h.p.deps.Operator.Apply = func(ops []config.Op) ([]config.Change, error) { applied = string(ops[0].Value); return nil, nil }
	items[1].take(h.p, func(text string) { h.p.source = text })
	h.tt.Frame()
	if applied != `"my-night"` || h.p.cfg.Theme != "my-night" || h.p.pal != pal {
		t.Fatal("custom selection did not persist")
	}
	c = *h.p.cfg
	c.Themes = []config.Theme{{ID: "my-night", Base: "flexoki-night", Colors: map[string]string{"accent": "#FFFFFF"}}}
	h.p.applyConfig(&c)
	h.tt.Frame()
	if h.p.pal == pal || h.p.pal.Accent != ui.Hex("#FFFFFF") {
		t.Fatal("config update did not rebuild palette")
	}
}
