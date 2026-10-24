//go:build soak && integration

package macos

import (
	"image"
	"image/draw"
	"strings"
	"testing"

	"github.com/Umupa/Kami/internal/soak"
	"github.com/egoist/mygo/ui"
)

func TestSoakVision(t *testing.T) {
	rounds := soak.Rounds(t, 200)
	var baseline, final soak.Sample
	for i := 1; i <= rounds; i++ {
		// Vary dimensions to exercise recognizer caches with multiple input sizes.
		src := ui.Render(func(c *ui.Context) {
			c.Root().Background(ui.Hex("#FFFFFF"))
			ui.Text(c, "Kami 压力测试").FontSize(28).TextColor(ui.Hex("#000000"))
		}, 480+(i%4)*120, 80+(i%3)*40, 2)
		rgba := image.NewRGBA(src.Bounds())
		draw.Draw(rgba, rgba.Bounds(), src, src.Bounds().Min, draw.Src)
		img, err := ImageFromRGBA(rgba)
		if err != nil {
			t.Fatal(err)
		}
		lines, err := RecognizeText(img)
		img.Release()
		if err != nil {
			t.Fatal(err)
		}
		var text string
		for _, line := range lines {
			text += line.Text
		}
		if !strings.Contains(text, "Kami") {
			t.Fatalf("recognition failed: %q", text)
		}
		if i%max(1, rounds/5) == 0 || i == rounds {
			final = soak.Measure(t, i)
			if i == rounds/5 {
				baseline = final
			}
		}
	}
	soak.Check(t, baseline, final)
	soak.Idle(t)
}
