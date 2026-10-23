package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

// trayIcon reduces the origami fox to pointed ears and cutout eyes. A template
// uses alpha alone so macOS can tint it on light, dark and selected menu bars.
// The request indicator stays separate from the face at 18 points.
func trayIcon(busy bool) []byte {
	const size, samples = 36, 4 // 18 points at 2x, with subpixel edge coverage.
	face := [][2]float64{{3, 3}, {13, 9}, {23, 9}, {33, 3}, {30, 19}, {18, 31}, {6, 19}}
	leftEye := [][2]float64{{8, 14}, {14, 17}, {15, 22}, {11, 19}}
	rightEye := [][2]float64{{28, 14}, {22, 17}, {21, 22}, {25, 19}}
	inside := func(x, y float64, polygon [][2]float64) bool {
		in := false
		previous := polygon[len(polygon)-1]
		for _, current := range polygon {
			if (current[1] > y) != (previous[1] > y) &&
				x < (previous[0]-current[0])*(y-current[1])/(previous[1]-current[1])+current[0] {
				in = !in
			}
			previous = current
		}
		return in
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			coverage := 0
			for sy := range samples {
				for sx := range samples {
					px := float64(x) + (float64(sx)+0.5)/samples
					py := float64(y) + (float64(sy)+0.5)/samples
					fox := inside(px, py, face) && !inside(px, py, leftEye) && !inside(px, py, rightEye)
					dx, dy := px-30.5, py-30.5
					if fox || busy && dx*dx+dy*dy < 9 {
						coverage++
					}
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{A: uint8(coverage * 255 / (samples * samples))})
		}
	}
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}
