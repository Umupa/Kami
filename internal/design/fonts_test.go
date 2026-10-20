package design

import (
	"testing"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/font/sfnt"
)

func TestBundledCaskaydia(t *testing.T) {
	for _, face := range []string{"Regular", "Bold", "Italic", "BoldItalic"} {
		t.Run(face, func(t *testing.T) {
			data, err := fontFiles.ReadFile("fonts/CaskaydiaCoveNerdFont-" + face + ".ttf")
			if err != nil {
				t.Fatal(err)
			}
			font, err := sfnt.Parse(data)
			if err != nil {
				t.Fatal(err)
			}
			name, err := font.Name(nil, sfnt.NameIDFamily)
			if err != nil || name != "CaskaydiaCove NF" {
				t.Fatalf("family = %q, error = %v", name, err)
			}
			for _, r := range "Aa09→\ue0b0" {
				glyph, err := font.GlyphIndex(nil, r)
				if err != nil || glyph == 0 {
					t.Errorf("missing glyph %U: %v", r, err)
				}
			}
		})
	}
	if err := LoadFont(); err != nil {
		t.Fatal(err)
	}
	for _, family := range ui.FontFamilies() {
		if family.Name == FontFamily && family.Registered {
			return
		}
	}
	t.Fatal("Caskaydia was not registered from the bundled font files")
}
