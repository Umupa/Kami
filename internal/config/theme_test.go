package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestThemeDefaults(t *testing.T) {
	inputs := []string{`{"version":1}`}
	for _, name := range RemovedThemes {
		inputs = append(inputs, fmt.Sprintf(`{"version":1,"theme":%q}`, name))
	}
	for _, input := range inputs {
		cfg, err := Decode([]byte(input))
		if err != nil {
			t.Fatalf("%s: %v", input, err)
		}
		if cfg.Theme != DefaultTheme {
			t.Errorf("%s: theme = %q, want %q", input, cfg.Theme, DefaultTheme)
		}
	}
	for _, name := range []string{"auto", "tokyo-night", "flexoki-light"} {
		cfg, err := Decode([]byte(fmt.Sprintf(`{"version":1,"theme":%q}`, name)))
		if err != nil || cfg.Theme != name {
			t.Errorf("theme %q decoded as %v, %v", name, cfg, err)
		}
	}
	_, err := Decode([]byte(`{"version":1,"theme":"flexoki-nigth"}`))
	if err == nil || !strings.Contains(err.Error(), "theme") {
		t.Errorf("a misspelled theme must report an error, got %v", err)
	}
}

func TestCustomThemes(t *testing.T) {
	valid := Theme{ID: "my-night", Name: "我的夜间", Base: "flexoki-night", Colors: map[string]string{"accent": "#DA702C", "code": "#a0af5480"}}
	data, _ := json.Marshal(Config{Version: Version, Theme: valid.ID, Themes: []Theme{valid}})
	c, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := c.CustomTheme(valid.ID)
	if !ok || got.Name != valid.Name || c.Theme != valid.ID {
		t.Fatalf("custom theme lost: %+v", c)
	}
	if _, ok := c.CustomTheme("missing"); ok {
		t.Error("found missing theme")
	}
	cases := []struct {
		name   string
		themes []Theme
		path   string
	}{
		{"invalid id", []Theme{{ID: "Bad", Base: valid.Base}}, "themes[0].id"},
		{"duplicate", []Theme{valid, valid}, "themes[1].id"},
		{"auto base", []Theme{{ID: valid.ID, Base: "auto"}}, "themes[0].base"},
		{"custom base", []Theme{valid, {ID: "child", Base: valid.ID}}, "themes[1].base"},
		{"unknown color", []Theme{{ID: valid.ID, Base: valid.Base, Colors: map[string]string{"accnet": "#FFFFFF"}}}, "themes[0].colors.accnet"},
	}
	for _, name := range append(append([]string{}, Themes...), RemovedThemes...) {
		cases = append(cases, struct {
			name   string
			themes []Theme
			path   string
		}{"reserved " + name, []Theme{{ID: name, Base: valid.Base}}, "themes[0].id"})
	}
	for _, value := range []string{"red", "#FFF", "#GG0000", "#1234567", "123456", " #123456"} {
		cases = append(cases, struct {
			name   string
			themes []Theme
			path   string
		}{value, []Theme{{ID: valid.ID, Base: valid.Base, Colors: map[string]string{"accent": value}}}, "themes[0].colors.accent"})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, _ := json.Marshal(Config{Version: Version, Themes: tc.themes})
			_, err := Decode(data)
			if err == nil || !strings.Contains(err.Error(), tc.path) {
				t.Fatalf("want %s, got %v", tc.path, err)
			}
		})
	}
}

func TestCustomThemeOps(t *testing.T) {
	data := []byte(`{"version":1,"themes":[]}`)
	after, _, err := ApplyOps(data, []Op{
		{Path: "themes[0]", Value: json.RawMessage(`{"id":"my-night","base":"flexoki-night","colors":{"accent":"#DA702C"}}`)},
		{Path: "theme", Value: json.RawMessage(`"my-night"`)},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	after, _, err = ApplyOps(after, []Op{{Path: "themes[0].colors.accent", Value: json.RawMessage(`"#FFFFFF"`)}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	c, err := Decode(after)
	if err != nil || c.Theme != "my-night" || c.Themes[0].Colors["accent"] != "#FFFFFF" {
		t.Fatalf("config %s: %v", after, err)
	}
}
