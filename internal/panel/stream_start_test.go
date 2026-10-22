package panel

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/session"
)

func TestStreamStartKeepsTopInEveryFrame(t *testing.T) {
	for _, reply := range []string{"", "开头。\n\n"} {
		t.Run(fmt.Sprintf("reply=%q", reply), func(t *testing.T) {
			h := newHarness(t, script{})
			h.p.place.maxHeight = 720
			h.p.source = strings.Repeat("The drag limit should come from the same calculation.\n", 40)
			h.p.sourceHeight = 285
			frames, overflows := 0, 0
			// Tester.Frame settles several renders. Observe each paint so a
			// one-frame jump cannot disappear before the assertion runs.
			h.tt = ui.NewTester(func(c *ui.Context) {
				h.p.view(c)
				c.Root().DrawOver(func(_ *ui.Painter, _ ui.Rect) {
					if h.p.sess == nil {
						return
					}
					frames++
					if h.p.list.scroll.MaxY > 0.5 {
						overflows++
					}
					if y := h.p.list.scroll.Y; y != 0 {
						t.Errorf("paint %d: growing reply jumped %.2fpt from its top", frames, y)
					}
				})
			}, 800, 720)
			h.tt.Frame()
			if h.p.panelH < float32(h.p.place.maxHeight) || h.p.least >= h.p.place.maxHeight {
				t.Fatal("setup must retain a tall window while its content fits below the limit")
			}
			h.p.sess = &session.Session{Turns: []session.Turn{{Reply: reply, Status: session.Streaming}}}
			// AppKit applies a pending height while the request starts, then
			// takes a few frames to catch up with the growing reply.
			h.tt.SetSize(800, h.p.height)
			h.tt.Advance(50 * time.Millisecond)
			h.p.sess.Last().Reply = "开头。\n\n正在翻译的第一段。\n\n"
			h.tt.Frame()
			h.tt.SetSize(800, h.p.height)
			h.tt.Advance(200 * time.Millisecond)
			h.tt.SetSize(800, h.p.height)
			if frames < 2 || overflows == 0 {
				t.Fatalf("did not exercise a temporarily constrained result: %d paints, %d overflows", frames, overflows)
			}
			if !h.tt.HasText("开头。") {
				t.Fatal("missing first paragraph after the window caught up")
			}
			if reply != "" {
				h.snapshotNow("stream-start-stable")
			}
		})
	}
}
