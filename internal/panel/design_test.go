package panel

import (
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const designReply = "## 会议纪要\n\n今天讨论了**三件事**，配置文件在 `~/Library/Application Support/cida-next/config.json`，详见[文档](https://example.com)。\n\n" +
	"| 事项 | 负责人 | 截止 |\n|---|---|---|\n| 面板改版 | 我 | 周五 |\n| Operator | Claude | 已完成 |\n\n" +
	"1. 先读配置\n2. 再改快捷键\n   - 截图改成 `option+s`\n\n" +
	"```go\nfunc main() {\n\t// 打个招呼\n\tfmt.Println(\"hello\", 42)\n}\n```\n\n" +
	"> 引用：Key 不会出现在任何输出里。\n"

func renderDesign(t *testing.T, name string, dark bool) {
	h := newHarness(t, script{pieces: []string{designReply}})
	h.p.place = placeOn(mygo.Rectangle{Width: 1440, Height: 1500})
	h.tt.SetSize(820, 1100)
	h.tt.SetDark(dark)
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
	h.tt.Key(0, ui.KeyTab)
	if h.p.source != "" || h.p.modes[h.p.modeIdx].ID != "translate" {
		t.Errorf("picking the second command: source %q, mode %s", h.p.source, h.p.modes[h.p.modeIdx].ID)
	}
	h.tt.Type("/op")
	h.tt.Key(0, ui.KeyEnter)
	if h.p.source != "/operator " {
		t.Errorf("Enter on /op gave %q, want /operator", h.p.source)
	}
}
