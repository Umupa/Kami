package panel

import (
	"math"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/design"
)

func TestManualResizeKeepsHeight(t *testing.T) {
	for _, state := range []string{"empty", "source", "result", "notes"} {
		t.Run(state, func(t *testing.T) {
			h := newHarness(t, script{pieces: []string{"ok"}})
			h.tt.Frame()
			switch state {
			case "source":
				h.p.source = "原文"
			case "result":
				h.tt.Click("原文")
				h.tt.Type("原文")
				h.tt.Key(0, ui.KeyEnter)
				h.drain()
			case "notes":
				h.enterNotes()
			}
			h.settle()
			base := h.p.height
			h.tt.SetSize(820, base)
			h.tt.Frame()
			h.sizes = nil
			for _, delta := range []int{40, 80, 120} {
				height := base + delta
				h.p.resized(mygo.Rectangle{Width: 820, Height: height})
				h.tt.SetSize(820, height)
				for range 3 {
					h.tt.Advance(16 * time.Millisecond)
					h.tt.Frame()
				}
				if len(h.sizes) != 0 || h.p.height != height {
					t.Fatalf("user height %d was overridden: height %d, requests %v", height, h.p.height, h.sizes)
				}
				if state == "empty" || state == "source" {
					bar, ok := h.tt.Find("执行")
					if !ok || math.Abs(float64(bar.Y+bar.H/2)-float64(float32(height)-design.ControlBarHeight/2)) > 2 {
						t.Fatalf("control bar %v is not at the bottom of height %d", bar, height)
					}
				}
			}
			if state == "empty" || state == "source" {
				h.snapshot("resized-" + state)
			}
		})
	}
}

func TestFitAfterEditingResizedSource(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	h.settle()
	initial := h.p.natural
	h.p.resized(mygo.Rectangle{Width: 820, Height: 400})
	h.tt.SetSize(820, 400)
	h.tt.Frame()
	h.sizes = nil
	h.tt.Click("原文")
	h.tt.Type("第一行\n第二行\n第三行")
	h.tt.Frame()
	if len(h.sizes) != 0 || h.p.height != 400 {
		t.Fatalf("editing resized the window: height %d, requests %v", h.p.height, h.sizes)
	}
	if h.p.natural <= initial || h.p.natural >= 400 {
		t.Fatalf("natural height %d did not track the edited source (initial %d)", h.p.natural, initial)
	}
	want := h.p.natural
	h.tt.Key(ui.Cmd|ui.Alt, ui.Key0)
	h.tt.Frame()
	h.tt.Frame()
	if h.p.fixedHeight != 0 || len(h.sizes) == 0 || h.sizes[len(h.sizes)-1].Height != want {
		t.Fatalf("fit did not request content height %d: fixed %d, requests %v", want, h.p.fixedHeight, h.sizes)
	}
}
