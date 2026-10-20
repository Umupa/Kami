package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestCaskaydiaOnly(t *testing.T) {
	for _, input := range []string{`{"version":1}`, `{"version":1,"font":"caskaydia"}`} {
		cfg, err := Decode([]byte(input))
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Font != "caskaydia" {
			t.Errorf("font = %q, want caskaydia", cfg.Font)
		}
	}
	for _, name := range []string{"jetbrains-mono", "sf-mono", "source-serif", "songti", "kaiti", "inter", "system", "unknown"} {
		t.Run(name, func(t *testing.T) {
			_, err := Decode([]byte(fmt.Sprintf(`{"version":1,"font":%q}`, name)))
			if err == nil || !strings.Contains(err.Error(), "仅支持 caskaydia") {
				t.Fatalf("unsupported font must report an error, got %v", err)
			}
		})
	}
}
