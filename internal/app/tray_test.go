package app

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"testing"
)

var updateTray = flag.Bool("update-tray", false, "rewrite the menu bar icon preview")

func TestTrayTemplateAndActivity(t *testing.T) {
	var icons []image.Image
	for _, busy := range []bool{false, true} {
		img, err := png.Decode(bytes.NewReader(trayIcon(busy)))
		if err != nil {
			t.Fatal(err)
		}
		if img.Bounds() != image.Rect(0, 0, 36, 36) {
			t.Fatalf("unexpected icon bounds: %v", img.Bounds())
		}
		for y := range 36 {
			for x := range 36 {
				r, g, b, a := img.At(x, y).RGBA()
				if r != 0 || g != 0 || b != 0 {
					t.Fatal("template icon contains baked-in color")
				}
				if (x == 0 || y == 0 || x == 35 || y == 35) && a != 0 {
					t.Fatal("icon touches the canvas edge")
				}
			}
		}
		icons = append(icons, img)
	}
	difference := 0
	for y := range 36 {
		for x := range 36 {
			_, _, _, idle := icons[0].At(x, y).RGBA()
			_, _, _, busy := icons[1].At(x, y).RGBA()
			if idle != busy {
				difference++
				if x < 27 || y < 27 || busy < idle {
					t.Fatal("activity indicator changes the fox face")
				}
			}
		}
	}
	if difference == 0 {
		t.Fatal("busy icon has no activity indicator")
	}
	if !*updateTray {
		return
	}
	// Left to right: light, dark, selected. Each pair is idle then busy.
	preview := image.NewRGBA(image.Rect(0, 0, 288, 64))
	for i, bg := range []color.RGBA{{240, 240, 240, 255}, {40, 40, 40, 255}, {38, 110, 220, 255}} {
		draw.Draw(preview, image.Rect(i*96, 0, (i+1)*96, 64), image.NewUniform(bg), image.Point{}, draw.Src)
		ink := color.Black
		if i > 0 {
			ink = color.White
		}
		for j, icon := range icons {
			r := image.Rect(i*96+6+j*48, 14, i*96+42+j*48, 50)
			draw.DrawMask(preview, r, image.NewUniform(ink), image.Point{}, icon, image.Point{}, draw.Over)
		}
	}
	if err := os.MkdirAll("testdata", 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create("testdata/tray-icons.png")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, preview); err != nil {
		t.Fatal(err)
	}
}
