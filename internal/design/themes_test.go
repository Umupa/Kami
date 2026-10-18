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
	if For("auto", true) != Themes[AutoDark] || For("auto", false) != Themes[AutoLight] {
		t.Error("auto does not follow the appearance")
	}
	if !Themes["tokyo-night"].Dark || Themes["catppuccin-latte"].Dark {
		t.Error("modes misread")
	}
}
