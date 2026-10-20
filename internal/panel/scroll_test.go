package panel

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Gloss/internal/session"
)

func TestConversationHoverKeepsPosition(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	h.p.sess = &session.Session{Turns: []session.Turn{
		{Reply: "## Earlier reply\n\nSome content.", Status: session.Done},
		{Ask: "Next question", Reply: strings.Repeat("More content.\n\n", 30), Status: session.Done},
	}}
	h.tt.Frame()
	h.p.list.ScrollTo(0, ui.Start)
	h.tt.Frame()
	h.tt.Move(0, 0)
	before, ok := h.tt.Find("Next question")
	if !ok {
		t.Fatal("missing next question")
	}
	heading, ok := h.tt.Find("Earlier reply")
	if !ok || heading.H == 0 {
		t.Fatal("earlier reply is not visible")
	}
	for range 3 {
		h.tt.Move(heading.X+20, heading.Y+10)
		if _, ok := h.tt.Find("复制 Markdown"); !ok {
			t.Fatal("hover did not show historical actions")
		}
		after, _ := h.tt.Find("Next question")
		if before != after {
			t.Fatalf("hover moved next reply: %v -> %v", before, after)
		}
		h.tt.Move(0, 0)
		if _, ok := h.tt.Find("复制 Markdown"); ok {
			t.Fatal("historical actions remained after leaving")
		}
		after, _ = h.tt.Find("Next question")
		if before != after {
			t.Fatalf("leaving moved next reply: %v -> %v", before, after)
		}
	}
	h.snapshot("scroll-stable-turns")
}

func TestMarkdownScrollPosition(t *testing.T) {
	for _, turns := range []int{1, 3} {
		t.Run(fmt.Sprint(turns), func(t *testing.T) {
			h := newHarness(t, script{})
			h.tt.Frame()
			h.p.sess = &session.Session{}
			var markers []string
			for turn := 0; turn < turns; turn++ {
				var text strings.Builder
				for section := 0; section < 12; section++ {
					marker := fmt.Sprintf("Turn %d section %d", turn, section)
					markers = append(markers, marker)
					fmt.Fprintf(&text, "## %s\n\nA paragraph with **bold** and `inline code`.\n\n- First item\n- Second item\n\n", marker)
					if section%3 == 0 {
						text.WriteString("| Left | Right |\n|---|---|\n| One | Two |\n\n```go\nfmt.Println(42)\n```\n\n")
					}
				}
				h.p.sess.Turns = append(h.p.sess.Turns, session.Turn{Ask: "next", Reply: text.String(), Status: session.Done})
			}
			h.tt.Frame()
			h.tt.Advance(2 * time.Second)
			if turns > 1 {
				// Cross the boundary into the previous reply under the pointer.
				h.p.list.ScrollTo(turns-1, ui.Start)
			} else {
				h.p.list.ScrollToEnd()
			}
			h.tt.Frame()
			for step := 0; step < 24; step++ {
				marker := ""
				var before ui.Rect
				for _, name := range markers {
					if r, ok := h.tt.Find(name); ok && r.H > 0 && r.Y > 150 && r.Y < 500 {
						marker, before = name, r
						break
					}
				}
				if marker == "" {
					t.Fatalf("step %d: no visible heading to track", step)
				}
				dy := float32(-30)
				if step >= 12 {
					dy = 30
				}
				h.tt.Scroll(400, 300, 0, dy)
				after, ok := h.tt.Find(marker)
				if !ok || math.Abs(float64(after.Y-before.Y+dy)) > 0.1 {
					t.Fatalf("step %d (%v): %q moved from %v to %v", step, dy, marker, before, after)
				}
				h.tt.Advance(time.Second)
				settled, _ := h.tt.Find(marker)
				if settled != after {
					t.Fatalf("step %d: drift %v -> %v", step, after, settled)
				}
			}
		})
	}
}

func TestShortStreamKeepsItsTop(t *testing.T) {
	for _, fixed := range []bool{false, true} {
		t.Run(fmt.Sprintf("fixed=%v", fixed), func(t *testing.T) {
			h := newHarness(t, script{})
			if fixed {
				h.p.fixedHeight = 600
			}
			h.tt.Frame()
			h.p.sess = &session.Session{Turns: []session.Turn{{Reply: "开头。\n\n", Status: session.Streaming}}}
			h.tt.Frame()
			h.tt.Advance(time.Second)
			before, ok := h.tt.Find("开头。")
			if !ok {
				t.Fatal("missing first paragraph")
			}
			for burst := 0; burst < 2; burst++ {
				h.p.sess.Turns[0].Reply += "- 第一项\n- 第二项\n- 第三项\n- 第四项\n"
				h.tt.Frame()
				for frame := 0; frame < 40; frame++ {
					h.tt.Advance(16 * time.Millisecond)
					after, ok := h.tt.Find("开头。")
					if !ok || math.Abs(float64(after.Y-before.Y)) > 0.1 {
						t.Fatalf("burst %d frame %d: top moved from %v to %v", burst, frame, before, after)
					}
					if _, ok := h.tt.Find("回到底部"); ok {
						t.Fatal("short content showed a spurious scroll-to-end button")
					}
					if !fixed && burst == 0 && frame == 4 {
						h.snapshotNow("stream-top-stable")
					}
				}
			}
			h.p.sess.Turns[0].Status = session.Done
			h.tt.Frame()
			h.tt.Advance(time.Second)
			after, _ := h.tt.Find("开头。")
			if math.Abs(float64(after.Y-before.Y)) > 0.1 {
				t.Fatalf("completion moved top: %v -> %v", before, after)
			}
		})
	}
}

func TestStreamingRespectsReadingPosition(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	var text strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&text, "段落 %d。\n\n", i)
	}
	h.p.sess = &session.Session{Turns: []session.Turn{{Reply: text.String(), Status: session.Streaming}}}
	h.tt.Frame()
	h.tt.Advance(2 * time.Second)
	if !h.p.list.AtEnd() {
		t.Fatal("long stream did not follow its end")
	}
	h.tt.Scroll(400, 300, 0, -200)
	if h.p.list.AtEnd() {
		t.Fatal("manual scroll did not stop following")
	}
	marker := ""
	var before ui.Rect
	for i := 0; i < 40; i++ {
		name := fmt.Sprintf("段落 %d。", i)
		if r, ok := h.tt.Find(name); ok && r.Y > 150 && r.Y < 400 && r.H > 0 {
			marker, before = name, r
			break
		}
	}
	if marker == "" {
		t.Fatal("no visible paragraph to track")
	}
	h.p.sess.Turns[0].Reply += strings.Repeat("追加内容。\n\n", 10)
	h.tt.Frame()
	for range 40 {
		h.tt.Advance(16 * time.Millisecond)
		after, _ := h.tt.Find(marker)
		if before != after || h.p.list.AtEnd() {
			t.Fatalf("stream moved reading position: %v -> %v", before, after)
		}
	}
	if err := h.tt.Click("回到底部"); err != nil {
		t.Fatal(err)
	}
	h.tt.Advance(time.Second)
	if !h.p.list.AtEnd() {
		t.Fatal("latest button did not resume following")
	}
	// A new request must not inherit the previous conversation's scroll extent.
	h.p.source = "new request"
	h.p.run()
	h.drain()
	h.tt.Advance(time.Second)
	if h.p.list.scroll.Y != 0 || h.p.list.scroll.MaxY > 0.5 {
		t.Fatalf("new conversation retained old scrolling: %+v", h.p.list.scroll)
	}
}
