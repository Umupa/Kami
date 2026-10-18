package panel

import (
	"github.com/egoist/mygo"

	"github.com/Umupa/cida-next/internal/design"
)

// placement is where the panel shows on a display: centered, its top edge at
// a fifth of the visible height, growing down to at most 70% of it.
type placement struct {
	x, top    int
	maxHeight int
}

func placeOn(work mygo.Rectangle) placement {
	return placement{
		x:         work.X + (work.Width-design.PanelWidth)/2,
		top:       work.Y + int(float64(work.Height)*design.PanelTopRatio),
		maxHeight: int(float64(work.Height) * design.PanelMaxRatio),
	}
}

// bounds returns the window's bounds for a content height.
func (pl placement) bounds(height int) mygo.Rectangle {
	return mygo.Rectangle{X: pl.x, Y: pl.top, Width: design.PanelWidth, Height: max(min(height, pl.maxHeight), 1)}
}
