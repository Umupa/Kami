package capture

import (
	"flag"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var updateSnapshots = flag.Bool("update", false, "rewrite capture snapshots")

// visualPicker generates an article in memory.
// No window, live screen, or system input is used.
func visualPicker(t *testing.T, dark bool) (*Capturer, *ui.Tester) {
	t.Helper()
	bg, paper, text, edge := ui.Hex("#FDFDFB"), ui.Hex("#FFFFFF"), ui.Hex("#242422"), ui.Hex("#E8E8E3")
	if dark {
		bg, paper, text, edge = ui.Hex("#151513"), ui.Hex("#222220"), ui.Hex("#D0D0CA"), ui.Hex("#383835")
	}
	src := ui.Render(func(ctx *ui.Context) {
		ctx.Root().Background(bg)
		ui.Column(ctx).Absolute().Left(120).Top(210).Size(960, 330).Padding(42, 56).Gap(18).Background(paper).Border(1, edge).Radius(14).Children(func() {
			ui.Text(ctx, "Storage engine").FontSize(28).FontWeight(600).TextColor(text)
			ui.Text(ctx, "The new storage engine keeps every write in an append-only log and compacts it in the background, so reads never wait for a merge.").FontSize(18).LineHeight(1.5).TextColor(text)
			ui.Text(ctx, "Snapshots are taken without pausing writers. Each one records the log position it saw, and recovery replays the log from there.").FontSize(18).LineHeight(1.5).TextColor(text)
			ui.Text(ctx, "Benchmarks on a four-core machine show three times the real throughput of the previous engine while data stays consistent.").FontSize(18).LineHeight(1.5).TextColor(text)
		})
	}, 1200, 750, 2)
	c := &Capturer{state: selecting, bitmap: ui.NewBitmap(src), ink: darkScreen(src), dismiss: func() {}, display: mygo.Display{Bounds: mygo.Rectangle{Width: 1200, Height: 750}, WorkArea: mygo.Rectangle{Width: 1200, Height: 690}}}
	tt := ui.NewTester(c.view, 1200, 750)
	tt.SetScale(2)
	t.Cleanup(c.Close)
	return c, tt
}

func captureSnapshot(t *testing.T, tt *ui.Tester, name string) {
	t.Helper()
	if !*updateSnapshots {
		return
	}
	if err := os.MkdirAll("testdata", 0755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join("testdata", name+".png"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, tt.Image()); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureReferenceStates(t *testing.T) {
	for _, dark := range []bool{false, true} {
		name := "light"
		if dark {
			name = "dark"
		}
		t.Run(name, func(t *testing.T) {
			c, tt := visualPicker(t, dark)
			tt.Frame()
			tt.Advance(60 * time.Millisecond)
			captureSnapshot(t, tt, name+"-fade")
			tt.Advance(200 * time.Millisecond)
			captureSnapshot(t, tt, name+"-veiled")
			before := tt.Image().RGBAAt(10, 10)
			if !dark && (before.R < 247 || before.R > 250 || before.B < 242 || before.B > 247) {
				t.Fatalf("paper veil pixel: %v", before)
			}
			if dark && (before.R < 22 || before.R > 26) {
				t.Fatalf("ink veil pixel: %v", before)
			}
			tt.Press(164, 365)
			tt.Move(1034, 438)
			tt.Advance(200 * time.Millisecond)
			captureSnapshot(t, tt, name+"-lifted")
			if c.hint.Y != 138 {
				t.Fatalf("hint anchor: %v", c.hint)
			}
			x, y := int((c.hint.X+5)*2), int((c.hint.Y+c.hint.H/2)*2)
			visible := tt.Image().RGBAAt(x, y)
			tt.Move(600, 145)
			tt.Advance(200 * time.Millisecond)
			captureSnapshot(t, tt, name+"-hint-covered")
			hidden := tt.Image().RGBAAt(x, y)
			if hidden == visible {
				t.Fatal("hint did not fade when the selection touched it")
			}
			tt.Move(1034, 438)
			tt.Advance(200 * time.Millisecond)
			if restored := tt.Image().RGBAAt(x, y); restored != visible {
				t.Fatalf("hint did not return: %v vs %v", restored, visible)
			}
			tt.SetDark(true)
			tt.Frame()
			captureSnapshot(t, tt, name+"-dark-hint")
		})
	}
}

func TestRightClickAndHintClickBelongToPicker(t *testing.T) {
	c, tt, results := picker(t)
	tt.Frame()
	tt.RightClickAt(100, 100)
	if r := wait(t, results); !r.Canceled {
		t.Fatalf("right click: %+v", r)
	}
	c, tt, results = picker(t)
	tt.Frame()
	tt.Advance(200 * time.Millisecond)
	tt.Press(c.hint.X+c.hint.W/2, c.hint.Y+10)
	tt.Release(c.hint.X+c.hint.W/2, c.hint.Y+10)
	if r := wait(t, results); !r.Canceled {
		t.Fatalf("hint swallowed the click: %+v", r)
	}
}

func TestVeilFadeAtTenPercentIntervals(t *testing.T) {
	_, tt := visualPicker(t, false)
	tt.Frame()
	previous := tt.Image().RGBAAt(10, 10).B
	first := previous
	for range 10 {
		tt.Advance(12 * time.Millisecond)
		current := tt.Image().RGBAAt(10, 10).B
		if current > previous {
			t.Fatal("veil fade reversed direction")
		}
		previous = current
	}
	if previous >= first {
		t.Fatal("veil never faded in")
	}
}
