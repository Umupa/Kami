//go:build integration

package macos

import (
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/egoist/mygo/ui"
)

func TestCaptureAPIsAvailable(t *testing.T) {
	if err := loadCapture(); err != nil {
		t.Fatal(err)
	}
	for _, api := range []struct{ class, query, method string }{
		{"SCShareableContent", "respondsToSelector:", "getShareableContentExcludingDesktopWindows:onScreenWindowsOnly:completionHandler:"},
		{"SCScreenshotManager", "respondsToSelector:", "captureImageWithFilter:configuration:completionHandler:"},
		{"SCContentFilter", "instancesRespondToSelector:", "initWithDisplay:excludingWindows:"},
		{"SCStreamConfiguration", "instancesRespondToSelector:", "setCaptureResolution:"},
	} {
		if !objc.Send[bool](class(api.class), sel(api.query), sel(api.method)) {
			t.Errorf("unavailable API: %s.%s", api.class, api.method)
		}
	}
}

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

func TestWarmupRecognizesBothScripts(t *testing.T) {
	img := warmUpImage()
	if img == nil {
		t.Fatal("warmup image creation failed")
	}
	defer img.Release()
	lines, err := RecognizeText(img)
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for _, l := range lines {
		text += l.Text
	}
	if !strings.Contains(text, "截图翻译") || !strings.Contains(text, "Kami reads") {
		t.Fatalf("warmup text: %q", text)
	}
}

// TestWarmupSpeedsUpLaterRecognition guards the reason capture warms on every
// start: Vision unloads after seconds of idling, and a warm-up shortly before
// a different image still saves most of the load.
func TestWarmupSpeedsUpLaterRecognition(t *testing.T) {
	src := ui.Render(func(c *ui.Context) {
		ui.Column(c).Fill().Padding(24).Gap(18).Background(ui.Hex("#FFFFFF")).Children(func() {
			ui.Text(c, "Framing usually outlasts the warm-up").FontSize(28).TextColor(ui.Hex("#000000"))
			ui.Text(c, "框选通常比预热更久").FontSize(28).TextColor(ui.Hex("#000000"))
		})
	}, 900, 160, 2)
	rgba := image.NewRGBA(src.Bounds())
	draw.Draw(rgba, rgba.Bounds(), src, src.Bounds().Min, draw.Src)
	img, err := ImageFromRGBA(rgba)
	if err != nil {
		t.Fatal(err)
	}
	defer img.Release()
	recognize := func() time.Duration {
		start := time.Now()
		if _, err := RecognizeText(img); err != nil {
			t.Fatal(err)
		}
		return time.Since(start)
	}
	time.Sleep(10 * time.Second)
	cold := recognize()
	time.Sleep(10 * time.Second)
	WarmUpVision()
	time.Sleep(500 * time.Millisecond) // the user framing
	warm := recognize()
	t.Logf("cold %s, after warm-up %s", cold, warm)
	if warm > cold*7/10 {
		t.Errorf("warm-up saved too little: cold %s, warm %s", cold, warm)
	}
}

func TestImageRoundTripAndTopCrop(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 128, 96))
	for y := range 96 {
		for x := range 128 {
			col := color.RGBA{R: 240, A: 255}
			if y >= 48 {
				col = color.RGBA{B: 240, A: 255}
			}
			src.SetRGBA(x, y, col)
		}
	}
	img, err := ImageFromRGBA(src)
	if err != nil {
		t.Fatal(err)
	}
	defer img.Release()
	got := img.RGBA()
	if got.RGBAAt(50, 10) != (color.RGBA{R: 240, A: 255}) || got.RGBAAt(50, 80) != (color.RGBA{B: 240, A: 255}) {
		t.Fatal("image roundtrip changed orientation")
	}
	crop, err := img.Crop(Rect{W: 128, H: 32})
	if err != nil {
		t.Fatal(err)
	}
	defer crop.Release()
	if crop.RGBA().RGBAAt(50, 10) != (color.RGBA{R: 240, A: 255}) {
		t.Fatal("crop used a bottom-left origin")
	}
	if _, err := img.Crop(Rect{}); err == nil {
		t.Fatal("empty crop succeeded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RecognizeTextContext(ctx, img); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled recognition: %v", err)
	}
}
