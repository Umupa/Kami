package design

import (
	"github.com/Umupa/Kami/internal/config"
	"github.com/egoist/mygo/ui"
	"reflect"
	"slices"
	"testing"
)

func TestDeriveRoundTrip(t *testing.T) {
	for _, name := range ThemeNames {
		t.Run(name, func(t *testing.T) {
			p := Themes[name]
			if got := Derive(p, p.Colors()); *got != *p {
				t.Errorf("round trip differs: %+v", got)
			}
			keys := []string{}
			for key := range p.Colors() {
				keys = append(keys, key)
			}
			want := slices.Clone(config.ThemeColors)
			slices.Sort(keys)
			slices.Sort(want)
			if !slices.Equal(keys, want) {
				t.Errorf("colors %v, want %v", keys, want)
			}
		})
	}
}

func TestDeriveDependencies(t *testing.T) {
	base := Themes["flexoki-night"]
	before := *base
	got := Derive(base, map[string]string{"background": "#FFFFFF", "accent": "#123456", "operator": "#654321"})
	if got.Border == base.Border || got.Muted == base.Muted || got.Faint == base.Faint || got.AccentText != got.Background || got.Edge != got.Accent.Alpha(0.35) || got.OperatorFill != got.Operator.Alpha(0.12) {
		t.Fatal("derived colors did not follow sources")
	}
	got = Derive(base, map[string]string{"background": "#FFFFFF", "border": "#12345678"})
	if got.Border != ui.Hex("#12345678") || got.Colors()["border"] != "#12345678" {
		t.Error("explicit border lost")
	}
	got = Derive(base, map[string]string{"unknown": "#123456", "text": "#GGGGGG", "accent": "#123", "border": "bad"})
	if !reflect.DeepEqual(got, base) {
		t.Error("invalid input changed palette")
	}
	if *base != before {
		t.Error("base mutated")
	}
}

func TestContrast(t *testing.T) {
	if got := Contrast(ui.Hex("#000000"), ui.Hex("#FFFFFF")); got != 21 {
		t.Errorf("black/white: %v", got)
	}
	for _, p := range Themes {
		if Contrast(p.Text, p.Text) != 1 {
			t.Error("self contrast differs from 1")
		}
	}
	if Contrast(ui.Hex("#777777"), ui.Hex("#FFFFFF")) >= 4.5 {
		t.Error("WCAG threshold miscomputed")
	}
}
