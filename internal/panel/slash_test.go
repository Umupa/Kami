package panel

import (
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

// operatorTopicTitles are the topics the operator menu lists, in order.
var operatorTopicTitles = []string{
	"切换配色主题",
	"改颜色，或基于内置主题新建一套",
	"各模式用哪个模型，默认模型是哪个",
	"查看或修改全局快捷键",
	"查看模式和完整提示词，增删模式",
	"哪些系统权限还没给",
	"我的语言和外语",
	"登录时自动打开",
	"交给 Coding Agent 配置模型服务",
}

// operatorMenuFrames bounds how many frames a test draws while waiting for
// a menu.
const operatorMenuFrames = 12

// waitForOperatorMenu draws frames until the menu shows every text in want
// and is no longer waiting for the window.
func waitForOperatorMenu(t *testing.T, h *harness, want ...string) {
	t.Helper()
	for i := 0; i < operatorMenuFrames; i++ {
		h.tt.Frame()
		if h.p.menuHidden {
			continue
		}
		ok := true
		for _, s := range want {
			if !h.tt.HasText(s) {
				ok = false
				break
			}
		}
		if ok {
			return
		}
	}
	kind, items := h.p.menu(h.p.pal)
	t.Fatalf("after %d frames the menu (kind %d, %d items, hidden %v) shows no %q; source %q, texts %q",
		operatorMenuFrames, kind, len(items), h.p.menuHidden, want, h.p.source, h.tt.Texts())
}

// assertMenuClosed draws a frame and fails while a menu still shows.
func assertMenuClosed(t *testing.T, h *harness) {
	t.Helper()
	h.tt.Frame()
	if kind, items := h.p.menu(h.p.pal); kind != menuNone {
		t.Fatalf("menu still open: kind %d, %d items, source %q", kind, len(items), h.p.source)
	}
	// The guide repeats some topic titles as examples, so the menu's own
	// hint is what says whether a menu is drawn.
	if h.tt.HasText("↑↓ 选择") {
		t.Fatalf("closed menu still shows its hint: %q", h.tt.Texts())
	}
}

func TestOperatorMenuInputVariationsShowAllTopics(t *testing.T) {
	for _, tc := range []struct {
		name, input string
	}{
		{"bare", "/operator"},
		{"space", "/operator "},
		{"spaces", "/operator  "},
		{"tab", "/operator\t"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, script{})
			h.tt.Frame()
			h.tt.Click("原文")
			h.tt.Type(tc.input)
			waitForOperatorMenu(t, h, operatorTopicTitles...)
			if h.p.source != tc.input {
				t.Errorf("source = %q, want %q", h.p.source, tc.input)
			}
			kind, items := h.p.menu(h.p.pal)
			if kind != menuTopics || len(items) != len(operatorTopicTitles) {
				t.Errorf("menu kind %d with %d items, want %d topics", kind, len(items), len(operatorTopicTitles))
			}
		})
	}
}

func TestOperatorMenuFiltersTopics(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("/operator   快捷")
	waitForOperatorMenu(t, h, "查看或修改全局快捷键")

	kind, items := h.p.menu(h.p.pal)
	if kind != menuTopics || len(items) != 1 {
		t.Fatalf("menu kind %d with %d items, want the 快捷键 topic", kind, len(items))
	}
	for _, title := range operatorTopicTitles {
		if title == "查看或修改全局快捷键" {
			continue
		}
		if h.tt.HasText(title) {
			t.Errorf("filtering by 快捷 still shows %q", title)
		}
	}
}

func TestOperatorMenuRequestsShowNoTopics(t *testing.T) {
	for _, tc := range []struct{ name, input string }{
		{"request", "/operator 我的语言和外语分别是什么？"},
		{"commandPrefix", "/operatorx"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, script{})
			h.tt.Frame()
			h.tt.Click("原文")
			h.tt.Type(tc.input)
			for i := 0; i < operatorMenuFrames; i++ {
				h.tt.Frame()
			}
			if kind, _ := h.p.menu(h.p.pal); kind == menuTopics {
				t.Fatalf("a topics menu opened for %q", h.p.source)
			}
			for _, title := range operatorTopicTitles {
				if h.tt.HasText(title) {
					t.Fatalf("a topics menu shows %q for %q", title, h.p.source)
				}
			}
		})
	}
}

func TestOperatorMenuEnterWithoutArrowsShowsGuide(t *testing.T) {
	oh := newOperatorHarness(t, "never asked")
	oh.tt.Frame()
	oh.tt.Click("原文")
	oh.tt.Type("/operator")
	waitForOperatorMenu(t, oh.harness, operatorTopicTitles...)

	oh.tt.Key(0, ui.KeyEnter)
	oh.drain()
	if oh.p.sess == nil {
		t.Fatalf("Enter on bare /operator started no session; source %q", oh.p.source)
	}
	turn := oh.p.sess.Last()
	if !turn.Local || !strings.Contains(turn.Reply, "## Operator") || oh.system != "" {
		t.Errorf("local %v, asked the model %v, reply %q", turn.Local, oh.system != "", turn.Reply)
	}
	if oh.p.source != "/operator" {
		t.Errorf("source = %q, want the command unchanged", oh.p.source)
	}
	assertMenuClosed(t, oh.harness)
}

func TestOperatorMenuTabPicksTheme(t *testing.T) {
	oh := newOperatorHarness(t, "never asked")
	oh.tt.Frame()
	oh.tt.Click("原文")
	oh.tt.Type("/operator")
	waitForOperatorMenu(t, oh.harness, operatorTopicTitles...)

	oh.tt.Key(0, ui.KeyTab)
	oh.tt.Frame()
	if oh.p.source != "/theme " {
		t.Fatalf("Tab gave %q, want /theme ", oh.p.source)
	}
	if oh.system != "" {
		t.Error("picking the theme asked the model")
	}
	waitForOperatorMenu(t, oh.harness, "跟随系统")
}

func TestOperatorMenuArrowsPickTheme(t *testing.T) {
	oh := newOperatorHarness(t, "never asked")
	oh.tt.Frame()
	oh.tt.Click("原文")
	oh.tt.Type("/operator")
	waitForOperatorMenu(t, oh.harness, operatorTopicTitles...)

	// Down and Up end on the first row again; the arrows still count as
	// an explicit choice, so Enter takes it instead of showing the guide.
	oh.tt.Key(0, ui.KeyDown)
	oh.tt.Key(0, ui.KeyUp)
	oh.tt.Key(0, ui.KeyEnter)
	oh.tt.Frame()
	if oh.p.source != "/theme " {
		t.Fatalf("Down, Up, Enter gave %q, want /theme ", oh.p.source)
	}
	if oh.system != "" {
		t.Error("picking the theme asked the model")
	}
	waitForOperatorMenu(t, oh.harness, "跟随系统")
}

func TestOperatorMenuEscapeClosesTopics(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("/operator")
	waitForOperatorMenu(t, h, operatorTopicTitles...)

	h.tt.Key(0, ui.KeyEscape)
	assertMenuClosed(t, h)
	if h.p.source != "/operator" {
		t.Errorf("Escape changed the source to %q", h.p.source)
	}
	if h.p.sess != nil {
		t.Error("Escape ran the guide")
	}
}

func TestOperatorMenuEditClearsArrowChoice(t *testing.T) {
	oh := newOperatorHarness(t, "never asked")
	oh.tt.Frame()
	oh.tt.Click("原文")
	oh.tt.Type("/operator")
	waitForOperatorMenu(t, oh.harness, operatorTopicTitles...)

	oh.tt.Key(0, ui.KeyDown)
	oh.tt.Key(0, ui.KeyUp)
	// Delete the command's last letter and type it again: the menu is back
	// at bare /operator, but the arrows' choice was for text now gone.
	oh.tt.Key(0, ui.KeyBackspace)
	oh.tt.Type("r")
	waitForOperatorMenu(t, oh.harness, operatorTopicTitles...)
	if oh.p.source != "/operator" {
		t.Fatalf("after the edit the source is %q, want bare /operator", oh.p.source)
	}

	oh.tt.Key(0, ui.KeyEnter)
	oh.drain()
	if oh.p.sess == nil {
		t.Fatalf("Enter after editing started no session; source %q", oh.p.source)
	}
	turn := oh.p.sess.Last()
	if !turn.Local || !strings.Contains(turn.Reply, "## Operator") || oh.system != "" {
		t.Errorf("local %v, asked the model %v, reply %q", turn.Local, oh.system != "", turn.Reply)
	}
	assertMenuClosed(t, oh.harness)
}
