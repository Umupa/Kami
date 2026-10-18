package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// trayIcon draws the menu bar icon, black on transparent for macOS to tint:
// a speech bubble, the panel's answer to a selection. busy adds a dot at its
// tail, shown while a request runs.
func trayIcon(busy bool) []byte {
	const size = 36 // 18 points at 2x
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	ink := func(x, y int, a float64) {
		if a <= 0 {
			return
		}
		old := img.NRGBAAt(x, y).A
		img.SetNRGBA(x, y, color.NRGBA{A: max(old, uint8(math.Min(a, 1)*255))})
	}
	// A rounded rectangle outline, w pixels wide, anti-aliased by its signed
	// distance.
	cx, cy, hw, hh, r, w := 18.0, 16.0, 14.0, 10.0, 7.0, 3.0
	for y := range size {
		for x := range size {
			qx := math.Abs(float64(x)+0.5-cx) - hw + r
			qy := math.Abs(float64(y)+0.5-cy) - hh + r
			d := math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
			ink(x, y, 0.5-(math.Abs(d+w/2)-w/2))
		}
	}
	// The tail, below the left of the bubble.
	for y := 26; y < 32; y++ {
		for x := 10; x < 10+(32-y); x++ {
			ink(x, y, 1)
		}
	}
	// Two lines of text inside.
	for _, l := range [][3]int{{11, 25, 13}, {11, 20, 18}} {
		for x := l[0]; x < l[1]; x++ {
			for y := l[2]; y < l[2]+2; y++ {
				ink(x, y, 1)
			}
		}
	}
	if busy {
		for y := range size {
			for x := range size {
				ink(x, y, 3.5-math.Hypot(float64(x)-29.5, float64(y)-29.5))
			}
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
