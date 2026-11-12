package panel

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/design"
	"github.com/Umupa/Kami/internal/llm"
)

// A historical turn's actions float over its corner while the pointer is on
// it: turns keep only design.SpaceTurns between them, and no text moves.
func TestHistoryActionsFloat(t *testing.T) {
	client := clientFunc(func(_ context.Context, _ llm.Request, onText func(string)) error {
		onText("ok")
		return nil
	})
	h := newHarness(t, client)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("source text")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Click("追问")
	h.tt.Type("shorter")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Advance(2 * time.Second)

	ok, _ := h.tt.Find("ok")
	ask, _ := h.tt.Find("shorter")
	if gap := ask.Y - (ok.Y + ok.H); gap > design.SpaceTurns+2 {
		t.Errorf("%v between the turns: want about %v, no row kept for hidden actions", gap, design.SpaceTurns)
	}
	if _, shown := h.tt.Find("复制 Markdown"); shown {
		t.Error("a historical turn shows its actions without the pointer on it")
	}
	h.tt.Move(ok.X+ok.W/2, ok.Y+ok.H/2)
	h.tt.Advance(time.Second)
	if _, shown := h.tt.Find("复制 Markdown"); !shown {
		t.Errorf("texts %q: hovering a historical turn shows no actions", h.tt.Texts())
	}
	okAfter, _ := h.tt.Find("ok")
	askAfter, _ := h.tt.Find("shorter")
	if okAfter != ok || askAfter != ask {
		t.Errorf("hovering moved the text: %v → %v, %v → %v", ok, okAfter, ask, askAfter)
	}
	h.snapshotNow("history-hover")
}

// A result cut by scrolling on both sides fades out at both edges, through
// to the desktop under a translucent panel, not into a band of color.
func TestResultFadesWhereCut(t *testing.T) {
	long := strings.Repeat("一段回复。\n\n", 60)
	h := newHarness(t, script{pieces: []string{long}})
	h.tt.SetVibrancy(true)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("hello")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Advance(2 * time.Second)
	h.tt.Scroll(400, 300, 0, -400)
	h.tt.Advance(time.Second)
	if s := h.p.list.scroll; s.Y < 1 || s.Y > s.MaxY-1 {
		t.Fatalf("scrolled to %v of %v: want the middle", s.Y, s.MaxY)
	}
	h.snapshotNow("result-fade")
}
