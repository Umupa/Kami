package design

import (
	"slices"
	"testing"

	"github.com/Umupa/cida-next/internal/config"
)

func TestThemesMatchConfig(t *testing.T) {
	if len(ThemeNames) < 20 {
		t.Fatalf("loaded %d themes", len(ThemeNames))
	}
	want := append([]string{"auto"}, ThemeNames...)
	if !slices.Equal(config.Themes, want) {
		t.Errorf("config.Themes = %v\nwant %v", config.Themes, want)
	}
	var fonts []string
	for _, f := range Fonts {
		fonts = append(fonts, f.Name)
	}
	if !slices.Equal(config.Fonts, fonts) || FontNamed(config.Fonts[0]).Name != DefaultFont {
		t.Errorf("config.Fonts = %v, presets %v", config.Fonts, fonts)
	}
	if MinFontSize != config.MinFontSize || MaxFontSize != config.MaxFontSize || FontBody != config.DefaultFontSize {
		t.Error("font size limits differ between config and design")
	}
	if For("auto", true) != Themes[AutoDark] || For("auto", false) != Themes[AutoLight] {
		t.Error("auto does not follow the appearance")
	}
	if !Themes["tokyo-night"].Dark || Themes["catppuccin-latte"].Dark {
		t.Error("modes misread")
	}
}
