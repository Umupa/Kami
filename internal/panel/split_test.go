package panel

import (
	"fmt"
	"math"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/design"
)

func TestSourceTypingAfterSmallSplit(t *testing.T) {
	for _, height := range []float32{36, 40, 54, 55, 65} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			h := sourceSplitHarness(t)
			dragSourceTo(t, h, height)
			checkSourceTyping(t, h)
			if height == 40 {
				h.snapshot("source-min-height")
			}
		})
	}
}

func TestSourceSplitFollowsFontSize(t *testing.T) {
	h := sourceSplitHarness(t)
	dragSourceTo(t, h, 55)
	before := h.p.sourceBox.H
	for range 5 {
		h.tt.Key(ui.Cmd, ui.KeyEqual)
	}
	h.tt.Frame()
	if h.p.sz.Source <= design.FontSource {
		t.Fatal("the font size did not increase")
	}
	checkSourceTyping(t, h)
	h.snapshot("source-min-height-large-font")
	h.tt.Key(ui.Cmd, ui.Key0)
	h.tt.Frame()
	if h.p.sourceBox.H != before {
		t.Errorf("restoring the font left source height %g, want %g", h.p.sourceBox.H, before)
	}
	checkSourceTyping(t, h)
}

func sourceSplitHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t, script{pieces: []string{"原文区至少保留一整行，输入时文字位置保持稳定。"}})
	h.tt.SetDark(false)
	h.tt.Frame()
	if err := h.tt.Click("原文"); err != nil {
		t.Fatal(err)
	}
	h.tt.Type("root@app:/var/www/html# ")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	return h
}

func dragSourceTo(t *testing.T, h *harness, height float32) {
	t.Helper()
	r, ok := h.tt.Find("调整原文与结果的高度")
	if !ok {
		t.Fatal("no source divider")
	}
	y := r.Y + r.H/2
	to := y + height - h.p.sourceBox.H
	h.tt.Press(400, y)
	h.tt.Move(400, to)
	h.tt.Release(400, to)
	h.tt.Frame()
	if err := h.tt.Click("原文"); err != nil {
		t.Fatal(err)
	}
}

// Observe the rendered caret, rather than duplicating the height formula:
// every edit must leave the whole line inside the padded source viewport.
func checkSourceTyping(t *testing.T, h *harness) {
	t.Helper()
	if !h.tt.Focused("原文") {
		t.Fatal("source is not focused")
	}
	var firstY float32
	for i := range 6 {
		h.tt.Type("x")
		caret, active := h.tt.TextCaret()
		if !active {
			t.Fatal("source has no active caret")
		}
		r := h.p.sourceBox
		if caret.Y < r.Y+design.SpaceSourceY-0.01 || caret.Y+caret.H > r.Y+r.H-design.SpaceSourceY+0.01 {
			t.Errorf("edit %d: caret %+v does not fit inside source %+v with vertical padding %g", i, caret, r, float32(design.SpaceSourceY))
		}
		if i == 0 {
			firstY = caret.Y
		} else if math.Abs(float64(caret.Y-firstY)) > 0.01 {
			t.Errorf("edit %d moved the caret vertically: %g → %g", i, firstY, caret.Y)
		}
	}
}

// dividerInView fails unless the divider and the control bar are whole
// inside a panel of the given height.
func dividerInView(t *testing.T, h *harness, height float32) ui.Rect {
	t.Helper()
	r, ok := h.tt.Find("调整原文与结果的高度")
	if !ok || r.H < 6 || r.Y+r.H > height {
		t.Fatalf("divider %+v is not in a panel %g high (source %+v)", r, height, h.p.sourceBox)
	}
	for _, label := range []string{"执行", "追问"} {
		if b, ok := h.tt.Find(label); ok && b.Y+b.H > height+0.5 {
			t.Fatalf("%s %+v is below a panel %g high", label, b, height)
		}
	}
	return r
}

func dragDivider(h *harness, by float32) {
	r, _ := h.tt.Find("调整原文与结果的高度")
	y := r.Y + r.H/2
	h.tt.Press(400, y)
	h.tt.Move(400, y+by)
	h.tt.Release(400, y+by)
	h.tt.Frame()
	h.tt.Frame()
}

func TestDividerStaysInResizedWindow(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	h.p.source = "第一性原理"
	h.settle()
	h.p.resized(mygo.Rectangle{Width: 700, Height: 450})
	h.tt.SetSize(700, 450)
	h.tt.Frame()
	dragDivider(h, 1000)
	r := dividerInView(t, h, 450)
	if r.Y+r.H < 450-design.ControlBarHeight-2 {
		t.Errorf("the divider stopped at %+v, short of the bar at the bottom", r)
	}
	h.snapshot("split-resized-limit")
	// Dragging back finds the divider at once.
	before := h.p.sourceBox.H
	dragDivider(h, -100)
	if got := h.p.sourceBox.H; math.Abs(float64(got-(before-100))) > 1 {
		t.Errorf("dragging back 100 from %g gave %g", before, got)
	}
}

func TestDividerStaysWhenWindowShrinks(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	h.p.source = "第一性原理"
	h.settle()
	dragDivider(h, 400)
	dragged := h.p.sourceHeight
	h.p.fixedHeight = 300 // as the user's resize sets it
	h.tt.SetSize(700, 300)
	h.tt.Frame()
	h.tt.Frame()
	dividerInView(t, h, 300)
	// The source gives way, but keeps the user's height for a taller window.
	if h.p.sourceHeight != dragged {
		t.Errorf("the dragged height %g became %g", dragged, h.p.sourceHeight)
	}
	h.p.fixedHeight = 600
	h.tt.SetSize(700, 600)
	h.tt.Frame()
	h.tt.Frame()
	if got := h.p.sourceBox.H; math.Abs(float64(got-dragged)) > 1 {
		t.Errorf("in a taller window the source is %g, want the dragged %g", got, dragged)
	}
}

func TestDividerLeavesRoomForReply(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"译文"}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("第一性原理")
	h.settle()
	h.p.resized(mygo.Rectangle{Width: 700, Height: 450})
	h.tt.SetSize(700, 450)
	h.tt.Frame()
	dragDivider(h, 1000)
	h.tt.Click("原文")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Frame()
	h.tt.Frame()
	dividerInView(t, h, 450)
	if !h.tt.HasText("译文") {
		t.Error("the reply is not in view")
	}
	h.snapshot("split-resized-reply")
}
