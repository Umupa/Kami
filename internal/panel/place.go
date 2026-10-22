package panel

import (
	"github.com/egoist/mygo"

	"github.com/Umupa/Kami/internal/design"
)

// placement is where the panel shows on a display: centered, its top edge at
// a fifth of the visible height, growing down to at most 70% of it, unless
// the user moved or resized it.
type placement struct {
	x, top    int
	width     int
	maxHeight int
	work      mygo.Rectangle
}

func placeOn(work mygo.Rectangle) placement {
	return placement{
		x:         work.X + (work.Width-design.PanelWidth)/2,
		top:       work.Y + int(float64(work.Height)*design.PanelTopRatio),
		width:     design.PanelWidth,
		maxHeight: int(float64(work.Height) * design.PanelMaxRatio),
		work:      work,
	}
}

// within reports whether the panel's top left corner is on work.
func (pl placement) within(work mygo.Rectangle) bool {
	return pl.width > 0 && pl.x >= work.X && pl.x < work.X+work.Width && pl.top >= work.Y && pl.top < work.Y+work.Height
}

// bounds returns the window's bounds for a content height.
func (pl placement) bounds(height int) mygo.Rectangle {
	w := pl.width
	if w <= 0 {
		w = design.PanelWidth
	}
	return mygo.Rectangle{X: pl.x, Y: pl.top, Width: w, Height: max(min(height, pl.maxHeight), 1)}
}
