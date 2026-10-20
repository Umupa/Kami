package design

import (
	"embed"
	"fmt"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// FontFamily is the bundled family used by both the interface and code.
// Core Text supplies missing glyphs, such as Chinese and emoji.
const FontFamily = "CaskaydiaCove Nerd Font"

//go:embed fonts/*.ttf fonts/LICENSE
var fontFiles embed.FS

// LoadFont registers the embedded faces once, before any text is laid out.
// Failure is returned to the caller rather than substituting another family.
func LoadFont() error { return loadFont() }

var loadFont = sync.OnceValue(func() error {
	files, err := fontFiles.ReadDir("fonts")
	if err != nil {
		return err
	}
	for _, file := range files {
		if !strings.HasSuffix(file.Name(), ".ttf") {
			continue
		}
		data, err := fontFiles.ReadFile("fonts/" + file.Name())
		if err != nil {
			return err
		}
		if err := ui.RegisterFont(data, FontFamily); err != nil {
			return fmt.Errorf("load bundled font %s: %w", file.Name(), err)
		}
	}
	return nil
})

// Sizes are the type sizes for a body size: the others keep their ratio to
// it, so one setting scales the panel's text.
type Sizes struct {
	Caption, Label, Body, Source float32
}

// Font size limits, in points.
const (
	MinFontSize = 11
	MaxFontSize = 22
)

// SizesFor returns the sizes for a body size; zero is the default.
func SizesFor(body float32) Sizes {
	if body <= 0 {
		body = FontBody
	}
	body = max(MinFontSize, min(body, MaxFontSize))
	k := body / FontBody
	return Sizes{Caption: FontCaption * k, Label: FontLabel * k, Body: body, Source: FontSource * k}
}
