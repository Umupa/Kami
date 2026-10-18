package design

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// FontPreset is a choice of fonts: a family list for text, the families
// tried in order, with Chinese falling back to the last; and the family of
// code.
type FontPreset struct {
	Name, Label string
	Family      string
	Code        string
	// Mono tells a monospaced preset, whose code uses the text's family.
	Mono bool
	// files are fonts the app ships in resources/fonts, registered under
	// the family's first name when the preset is first used.
	files map[string]string
}

const monoCode = "CaskaydiaCove Nerd Font, JetBrains Mono, SF Mono, Menlo"

// Fonts are the presets config.json names, in the order menus list them.
var Fonts = []FontPreset{
	{Name: "caskaydia", Label: "CaskaydiaCove · Omarchy 的等宽字体", Family: "CaskaydiaCove Nerd Font, JetBrains Mono, SF Mono, PingFang SC", Code: monoCode, Mono: true,
		files: map[string]string{"JetBrains Mono": "JetBrainsMono.ttf"}},
	{Name: "jetbrains-mono", Label: "JetBrains Mono · 等宽", Family: "JetBrains Mono, PingFang SC", Code: "JetBrains Mono", Mono: true,
		files: map[string]string{"JetBrains Mono": "JetBrainsMono.ttf"}},
	{Name: "sf-mono", Label: "SF Mono · 系统等宽", Family: "SF Mono, Menlo, PingFang SC", Code: "SF Mono, Menlo", Mono: true},
	{Name: "cida-serif", Label: "Source Serif · 辞达的衬线", Family: "Source Serif 4, Songti SC", Code: monoCode,
		files: map[string]string{"Source Serif 4": "SourceSerif4.ttf", "JetBrains Mono": "JetBrainsMono.ttf"}},
	{Name: "songti", Label: "宋体 · 经典中文衬线", Family: "Songti SC, Georgia", Code: monoCode},
	{Name: "kaiti", Label: "楷体 · 书写感", Family: "Kaiti SC, Georgia", Code: monoCode},
	{Name: "inter", Label: "Inter · 辞达的界面字体", Family: "Inter, PingFang SC", Code: monoCode,
		files: map[string]string{"Inter": "Inter.ttf", "JetBrains Mono": "JetBrainsMono.ttf"}},
	{Name: "system", Label: "苹方 · 系统字体", Family: "", Code: "SF Mono, Menlo"},
}

// DefaultFont is the preset when config.json names none.
const DefaultFont = "caskaydia"

// FontNamed returns a preset by name, the default for an unknown one.
func FontNamed(name string) FontPreset {
	for _, f := range Fonts {
		if f.Name == name {
			return f
		}
	}
	return Fonts[0]
}

var (
	registerMu sync.Mutex
	registered = map[string]bool{}
)

// Use registers the fonts a preset ships, once, and returns it. A font that
// cannot be read is skipped: the family list falls back to the next.
func (f FontPreset) Use() FontPreset {
	registerMu.Lock()
	defer registerMu.Unlock()
	if len(f.files) == 0 {
		return f
	}
	dir, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return f
	}
	for family, file := range f.files {
		if registered[family] {
			continue
		}
		registered[family] = true
		data, err := os.ReadFile(filepath.Join(dir, "fonts", file))
		if err == nil {
			_ = ui.RegisterFont(data, family)
		}
	}
	return f
}

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
