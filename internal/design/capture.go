package design

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// Capture follows Cida's capture surface rather than the conversation theme.
// The veil follows the frozen image; the hint follows the app's appearance.
const (
	CaptureRadius     = 8
	CaptureFade       = 120 * time.Millisecond
	CaptureHintSize   = 12.5
	CaptureHintHeight = 34
)

var (
	CapturePaperVeil = ui.Hex("#F7F6F1").Alpha(0.72)
	CaptureInkVeil   = ui.Hex("#1B1A17").Alpha(0.45)
	CapturePaperEdge = ui.Hex("#00000012")
	CaptureInkEdge   = ui.Hex("#FFFFFF24")
	CaptureContact   = ui.Hex("#00000014")
	CaptureAmbient   = ui.Hex("#1A1A1830")
)

// CaptureHint returns the pill's paper, text, border and two shadows.
func CaptureHint(dark bool) (ui.Color, ui.Color, ui.Color, ui.Color, ui.Color) {
	if dark {
		return ui.Hex("#262624"), ui.Hex("#C2C2BA"), ui.Hex("#FFFFFF1F"), ui.Hex("#00000040"), ui.Hex("#00000073")
	}
	return ui.Hex("#FFFFFF"), ui.Hex("#4E4E49"), CapturePaperEdge, CaptureContact, CaptureAmbient
}
