package operator

import (
	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/design"
	"strings"
	"testing"
)

func TestThemePrompt(t *testing.T) {
	palettes := map[string]map[string]string{}
	for _, name := range design.ThemeNames {
		palettes[name] = design.Themes[name].Colors()
	}
	palettes["my-night"] = design.Derive(design.Themes["flexoki-night"], map[string]string{"accent": "#123456"}).Colors()
	for _, tc := range []struct {
		theme    string
		dark     bool
		resolved string
	}{
		{"my-night", false, "my-night"}, {"auto", false, design.AutoLight}, {"auto", true, design.AutoDark},
	} {
		t.Run(tc.theme+tc.resolved, func(t *testing.T) {
			s := State{Config: &config.Config{Theme: tc.theme, Model: &config.Model{APIKey: "secret-test-key"}}, File: []byte(`{"model":{"apiKey":"secret-test-key"},"models":[{"apiKey":"secret-test-key"}]}`), Palettes: palettes, Dark: tc.dark}
			prompt := System(s)
			section := themeSection(s)
			if strings.Contains(prompt, "secret-test-key") {
				t.Fatal("API key exposed")
			}
			if !strings.Contains(section, "Theme in effect: "+tc.resolved) {
				t.Error("wrong active theme")
			}
			for _, key := range config.ThemeColors {
				if !strings.Contains(section, "- "+key+": "+palettes[tc.resolved][key]) {
					t.Errorf("missing color %s", key)
				}
			}
			for _, name := range design.ThemeNames {
				if !strings.Contains(section, "- "+name+", ") {
					t.Errorf("missing theme %s", name)
				}
			}
			if strings.Index(prompt, "## Custom themes") > strings.Index(prompt, "Permissions:") {
				t.Error("theme section after permissions")
			}
		})
	}
}
