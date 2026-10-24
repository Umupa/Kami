package design

import (
	"bytes"
	"flag"
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the site's copy of the themes")

// siteThemes is the site's copy of the themes, the colors its demo panel
// draws with.
const siteThemes = "../../site/themes.js"

// TestSiteThemes keeps the demo panel on the site in the app's colors;
// go test ./internal/design -update writes the copy again.
func TestSiteThemes(t *testing.T) {
	var b bytes.Buffer
	b.WriteString("// Kami's built-in themes as internal/design resolves them: each palette's\n")
	b.WriteString("// colors by role, derived ones included. Written by\n")
	b.WriteString("// `go test ./internal/design -update`; do not edit.\n")
	b.WriteString("window.KAMI_THEMES = [\n")
	for _, name := range ThemeNames {
		p := Themes[name]
		colors := p.Colors()
		var parts []string
		for _, key := range slices.Sorted(maps.Keys(colors)) {
			parts = append(parts, fmt.Sprintf("%s: %q", key, colors[key]))
		}
		fmt.Fprintf(&b, "  { name: %q, dark: %t, colors: { %s } },\n", name, p.Dark, strings.Join(parts, ", "))
	}
	b.WriteString("];\n")

	if *update {
		if err := os.WriteFile(siteThemes, b.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(siteThemes)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, b.Bytes()) {
		t.Errorf("%s is out of date; run go test ./internal/design -update", siteThemes)
	}
}
