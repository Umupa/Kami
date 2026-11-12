package panel

import (
	"math"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/design"
)

// The run button and the pin keep their place as a result comes, and the
// control bar is the one row at the bottom: the follow-up, the latest
// turn's actions and the pin share it.
func TestBarKeepsItsPlace(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"结果"}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("原文")
	h.tt.Frame()
	run, ok := h.tt.Find("执行")
	if !ok {
		t.Fatal("no run button before a result")
	}
	pin, _ := h.tt.Find("固定面板 ⌘⇧P")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Frame()
	if h.tt.HasText("执行") {
		t.Error("the run button shows with a result")
	}
	bottom := float32(h.p.height)
	after, _ := h.tt.Find("固定面板 ⌘⇧P")
	if math.Abs(float64(after.X-pin.X)) > 0.5 {
		t.Errorf("the pin moved from %v to %v", pin, after)
	}
	if mid := bottom - design.ControlBarHeight/2; math.Abs(float64(after.Y+after.H/2-mid)) > 2 {
		t.Errorf("the pin %v is not in the bar at the bottom of %g", after, bottom)
	}
	follow, ok := h.tt.Find("追问")
	copyIcon, copyOK := h.tt.Find("复制 Markdown ⌘C")
	if !ok || !copyOK {
		t.Fatal("the follow-up or the copy icon is missing")
	}
	for name, r := range map[string]ui.Rect{"follow-up": follow, "copy": copyIcon} {
		if math.Abs(float64(r.Y+r.H/2-(after.Y+after.H/2))) > 2 {
			t.Errorf("the %s at %v is not in the pin's row %v", name, r, after)
		}
	}
	// Editing the source brings the run button back where it was.
	h.tt.Click("原文")
	h.tt.Type("，改了")
	h.tt.Frame()
	again, ok := h.tt.Find("执行")
	if !ok || math.Abs(float64(again.X+again.W-(run.X+run.W))) > 0.5 {
		t.Errorf("the run button for an edited source is at %v, was %v", again, run)
	}
	if moved, _ := h.tt.Find("固定面板 ⌘⇧P"); math.Abs(float64(moved.X-pin.X)) > 0.5 {
		t.Errorf("the pin moved to %v with the run button", moved)
	}
	h.snapshot("result-stale")
}

// A follow-up that wraps grows the bar; the pin keeps to its bottom.
func TestBarGrowsWithFollowUp(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"结果"}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("原文")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Frame()
	pin, _ := h.tt.Find("固定面板 ⌘⇧P")
	gap := float32(h.p.height) - pin.Y
	one, _ := h.tt.Find("追问")
	h.tt.Click("追问")
	h.p.follow = strings.Repeat("很长的追问，", 60)
	h.tt.Frame()
	h.tt.Frame()
	more, _ := h.tt.Find("追问")
	if more.H < 2*one.H {
		t.Errorf("the follow-up did not wrap: %v, one line %v", more, one)
	}
	after, _ := h.tt.Find("固定面板 ⌘⇧P")
	if math.Abs(float64(float32(h.p.height)-after.Y-gap)) > 2 {
		t.Errorf("the pin moved from %v to %v in a panel %d high as the follow-up wrapped", pin, after, h.p.height)
	}
	h.snapshot("follow-up-wrapped")
}

// The latest turn's actions are in the bar, not under its reply.
func TestLatestActionsInBar(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"结果"}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("原文")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Frame()
	reply, _ := h.tt.Find("结果")
	retry, ok := h.tt.Find("重新生成")
	follow, _ := h.tt.Find("追问")
	if !ok || retry.X < follow.X+follow.W-1 {
		t.Errorf("regenerate at %v is not right of the follow-up %v", retry, follow)
	}
	if retry.Y < reply.Y+reply.H+20 {
		t.Errorf("regenerate at %v sits under the reply %v", retry, reply)
	}
	if h.tt.HasText("◐") || h.tt.HasText("切换") {
		t.Errorf("the theme button or the ⌘J hint shows: %q", h.tt.Texts())
	}
}
