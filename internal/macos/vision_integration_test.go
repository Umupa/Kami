//go:build integration

package macos

import (
	"image"
	"image/draw"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestRecognizeText reads text rendered offscreen: tests never capture the
// live screen.
func TestRecognizeText(t *testing.T) {
	src := ui.Render(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(24).Gap(18).Background(ui.Hex("#FFFFFF")).Children(func() {
			ui.Text(c, "The quick brown fox jumps over the lazy dog").FontSize(28).TextColor(ui.Hex("#000000"))
			ui.Text(c, "敏捷的棕色狐狸跳过了懒狗").FontSize(28).TextColor(ui.Hex("#000000"))
		})
	}, 900, 160, 2)
	rgba := image.NewRGBA(src.Bounds())
	draw.Draw(rgba, rgba.Bounds(), src, src.Bounds().Min, draw.Src)
	img, err := ImageFromRGBA(rgba)
	if err != nil {
		t.Fatal(err)
	}
	defer img.Release()

	for _, round := range []string{"cold", "warm"} {
		start := time.Now()
		lines, err := RecognizeText(img)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %v %+v", round, time.Since(start), lines)
		var texts []string
		for _, l := range lines {
			texts = append(texts, l.Text)
		}
		got := strings.Join(texts, "\n")
		if !strings.Contains(got, "quick brown fox") || !strings.Contains(got, "狐狸") {
			t.Errorf("recognized %q", got)
		}
		if len(lines) == 2 && lines[0].Box.Y > lines[1].Box.Y {
			t.Error("lines are not in reading order")
		}
	}
}
