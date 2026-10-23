package panel

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/session"
)

func TestRetainedHeightStreamKeepsTopInEveryPaint(t *testing.T) {
	for _, restart := range []string{"new-session", "retry"} {
		t.Run(restart, func(t *testing.T) {
			h := newHarness(t, script{})
			h.p.place.maxHeight = 720
			h.p.source = strings.Repeat("Original material.\n", 30)
			h.p.sourceHeight = 100
			h.p.sess = &session.Session{Turns: []session.Turn{{
				Reply: strings.Repeat("Previous long reply.\n\n", 50), Status: session.Done,
			}}}
			watch, paints, jumps, controlJumps := false, 0, 0, 0
			var top, pinX float32
			h.tt = ui.NewTester(func(c *ui.Context) {
				h.p.view(c)
				// Frame settles multiple renders. Check the pixels' geometry
				// at each paint, including temporary overflow before relayout.
				c.Root().DrawOver(func(_ *ui.Painter, _ ui.Rect) {
					if !watch {
						return
					}
					paints++
					r, ok := h.tt.Find("Anchor")
					if !ok || math.Abs(float64(r.Y-top)) > 0.5 || h.p.list.scroll.Y > 0.5 {
						jumps++
						if jumps == 1 {
							t.Errorf("paint %d: short reply moved from %.2f to %.2f (found=%v, scroll=%.2f, filler=%.2f, natural=%d, follow=%v)",
								paints, top, r.Y, ok, h.p.list.scroll.Y, h.p.fillerH, h.p.natural, h.p.list.FollowEnd)
						}
					}
					pin, pinOK := h.tt.Find("固定面板 ⌘⇧P")
					_, fitOK := h.tt.Find("适应内容 ⌥⌘0")
					if !pinOK || !fitOK || math.Abs(float64(pin.X-pinX)) > 0.5 {
						controlJumps++
						if controlJumps == 1 {
							t.Errorf("paint %d: retained-height controls changed (pin=%v x=%.2f want=%.2f fit=%v natural=%d filler=%.2f)",
								paints, pinOK, pin.X, pinX, fitOK, h.p.natural, h.p.fillerH)
						}
					}
				})
			}, 800, 720)
			h.tt.Frame()
			h.tt.Advance(time.Second)
			if h.p.least != 720 || !h.p.list.AtEnd() || h.p.list.scroll.Y <= 0 {
				t.Fatal("setup must reach the height limit and follow a long reply")
			}
			if restart == "new-session" {
				h.p.sess = &session.Session{Turns: []session.Turn{{Status: session.Streaming}}}
				h.p.list = newList()
			} else {
				h.p.sess.Retry()
			}
			h.p.sess.Last().Reply = "## Anchor\n\nFirst paragraph stays here.\n\n"
			h.tt.Frame()
			h.tt.Advance(time.Second)
			r, ok := h.tt.Find("Anchor")
			if !ok || h.p.height != 720 || h.p.fillerH <= 0 {
				t.Fatal("short reply must retain the window with spare room below it")
			}
			top, watch = r.Y, true
			pin, ok := h.tt.Find("固定面板 ⌘⇧P")
			if !ok {
				t.Fatal("missing pin button")
			}
			pinX = pin.X
			if _, ok := h.tt.Find("适应内容 ⌥⌘0"); !ok {
				t.Fatal("retained spare height must offer fit-to-content")
			}
			for range 8 {
				h.p.sess.Last().Reply += "- Existing words must stay anchored when another line arrives.\n"
				h.tt.Frame()
				for range 25 {
					h.tt.Advance(16 * time.Millisecond)
				}
			}
			// Sending a follow-up queues ScrollToEnd even when the added
			// turn still fits. It must not bypass the same top anchor.
			h.p.sess.Last().Status = session.Done
			h.p.sess.FollowUp("A short follow-up")
			h.p.sess.Last().Reply = "Another short reply."
			h.p.list.ScrollToEnd()
			h.tt.Frame()
			h.tt.Advance(time.Second)
			watch = false
			if paints < 8 || h.p.fillerH <= 0 {
				t.Fatalf("did not exercise growth within spare room: paints=%d filler=%.2f", paints, h.p.fillerH)
			}
			if restart == "new-session" {
				h.snapshotNow("retained-stream-top-stable")
			}
			// Keeping short replies anchored must not disable following
			// when this same reply really outgrows the available space.
			h.p.sess.Last().Reply += strings.Repeat("More content.\n\n", 50)
			h.tt.Frame()
			h.tt.Advance(3 * time.Second)
			if !h.p.list.FollowEnd || !h.p.list.AtEnd() || h.p.list.scroll.Y <= 0 {
				t.Fatal("long reply did not resume following its end")
			}
		})
	}
}
