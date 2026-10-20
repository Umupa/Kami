package mode

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Umupa/Gloss/internal/config"
)

func TestResolveBuiltins(t *testing.T) {
	modes, err := Resolve(&config.Config{})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := []struct {
		id     string
		name   string
		input  config.Input
		output config.Output
	}{
		{"translate", "翻译", config.InputMaterial, config.OutputAuto},
		{"improve", "润色", config.InputMaterial, config.OutputAuto},
		{"prompt", "提示词", config.InputMaterial, config.OutputMarkdown},
		{"ask", "问答", config.InputInstruction, config.OutputMarkdown},
	}
	if len(modes) != len(want) {
		t.Fatalf("len(Resolve) = %d, want %d", len(modes), len(want))
	}
	for i, w := range want {
		m := modes[i]
		if m.ID != w.id {
			t.Errorf("modes[%d].ID = %q, want %q", i, m.ID, w.id)
		}
		if m.Name != w.name {
			t.Errorf("modes[%d].Name = %q, want %q", i, m.Name, w.name)
		}
		if m.Input != w.input {
			t.Errorf("modes[%d].Input = %q, want %q", i, m.Input, w.input)
		}
		if m.Output != w.output {
			t.Errorf("modes[%d].Output = %q, want %q", i, m.Output, w.output)
		}
		if strings.TrimSpace(m.Prompt) == "" {
			t.Errorf("modes[%d].Prompt is empty", i)
		}
		if m.Model != "" {
			t.Errorf("modes[%d].Model = %q, want empty without models", i, m.Model)
		}
	}
}

func TestResolveBuiltinOverrides(t *testing.T) {
	base := Mode{
		ID:     "prompt",
		Name:   "提示词",
		Desc:   "把模糊的需求梳理成结构化的提示词",
		Prompt: promptPrompt,
		Input:  config.InputMaterial,
		Output: config.OutputMarkdown,
	}
	cases := []struct {
		name  string
		entry config.Mode
		want  Mode
	}{
		{
			name:  "name",
			entry: config.Mode{ID: "prompt", Name: "自定义名字"},
			want: func() Mode {
				m := base
				m.Name = "自定义名字"
				return m
			}(),
		},
		{
			name:  "prompt",
			entry: config.Mode{ID: "prompt", Prompt: "自定义提示词"},
			want: func() Mode {
				m := base
				m.Prompt = "自定义提示词"
				return m
			}(),
		},
		{
			name:  "input",
			entry: config.Mode{ID: "prompt", Input: config.InputInstruction},
			want: func() Mode {
				m := base
				m.Input = config.InputInstruction
				return m
			}(),
		},
		{
			name:  "output",
			entry: config.Mode{ID: "prompt", Output: config.OutputPlain},
			want: func() Mode {
				m := base
				m.Output = config.OutputPlain
				return m
			}(),
		},
		{
			name:  "model",
			entry: config.Mode{ID: "prompt", Model: "chosen"},
			want: func() Mode {
				m := base
				m.Model = "chosen"
				return m
			}(),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			modes, err := Resolve(&config.Config{Modes: []config.Mode{tc.entry}})
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if len(modes) != 1 {
				t.Fatalf("len(Resolve) = %d, want 1", len(modes))
			}
			if got := modes[0]; got != tc.want {
				t.Errorf("Resolve = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func TestResolveCustomModeDefaults(t *testing.T) {
	cfg := &config.Config{
		DefaultModel: "fast",
		Models:       []config.Model{{Name: "fast"}, {Name: "strong"}},
		Modes:        []config.Mode{{ID: "summary", Name: "总结", Prompt: "总结文本"}},
	}
	modes, err := Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := Mode{
		ID:     "summary",
		Name:   "总结",
		Desc:   "总结文本",
		Prompt: "总结文本",
		Input:  config.InputMaterial,
		Output: config.OutputMarkdown,
		Model:  "fast",
	}
	if len(modes) != 1 {
		t.Fatalf("len(Resolve) = %d, want 1", len(modes))
	}
	if got := modes[0]; got != want {
		t.Errorf("Resolve = %+v, want %+v", got, want)
	}
}

func TestResolveCustomModeProblems(t *testing.T) {
	cfg := &config.Config{Modes: []config.Mode{
		{ID: "one"},
		{ID: "two", Name: "Two"},
		{ID: "three", Prompt: "Do the thing."},
	}}
	wantPaths := []string{
		"modes[0].name",
		"modes[0].prompt",
		"modes[1].prompt",
		"modes[2].name",
	}

	modes, err := Resolve(cfg)
	if err == nil {
		t.Fatal("Resolve: err = nil, want a config error")
	}
	if modes != nil {
		t.Errorf("Resolve: modes = %v, want nil on error", modes)
	}
	var resolveErr *config.Error
	if !errors.As(err, &resolveErr) {
		t.Fatalf("Resolve error is %T, want *config.Error", err)
	}
	if got := problemPaths(resolveErr); !slices.Equal(got, wantPaths) {
		t.Errorf("Resolve problem paths = %v, want %v", got, wantPaths)
	}

	checkErr := Check(cfg)
	if checkErr == nil {
		t.Fatal("Check: err = nil, want a config error")
	}
	var checkConfigErr *config.Error
	if !errors.As(checkErr, &checkConfigErr) {
		t.Fatalf("Check error is %T, want *config.Error", checkErr)
	}
	if got := problemPaths(checkConfigErr); !slices.Equal(got, wantPaths) {
		t.Errorf("Check problem paths = %v, want %v", got, wantPaths)
	}
	if got, want := checkErr.Error(), err.Error(); got != want {
		t.Errorf("Check error = %q, want Resolve error %q", got, want)
	}
}

func problemPaths(err *config.Error) []string {
	paths := make([]string, len(err.Problems))
	for i, p := range err.Problems {
		paths[i] = p.Path
	}
	return paths
}

func TestResolveListedSubset(t *testing.T) {
	cfg := &config.Config{Modes: []config.Mode{
		{ID: "ask"},
		{ID: "translate"},
		{ID: "prompt"},
	}}
	modes, err := Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantIDs := []string{"ask", "translate", "prompt"}
	if len(modes) != len(wantIDs) {
		t.Fatalf("len(Resolve) = %d, want %d", len(modes), len(wantIDs))
	}
	for i, want := range wantIDs {
		if modes[i].ID != want {
			t.Errorf("modes[%d].ID = %q, want %q", i, modes[i].ID, want)
		}
	}
	if modes[0].Input != config.InputInstruction || modes[0].Output != config.OutputMarkdown {
		t.Errorf("ask = %+v, want the built-in input and output", modes[0])
	}
	if modes[1].Input != config.InputMaterial || modes[1].Output != config.OutputAuto {
		t.Errorf("translate = %+v, want the built-in input and output", modes[1])
	}
}

func TestResolveModel(t *testing.T) {
	fastStrong := []config.Model{{Name: "fast"}, {Name: "strong"}}
	cases := []struct {
		name string
		cfg  *config.Config
		want []string
	}{
		{
			name: "built-in preferences",
			cfg:  &config.Config{DefaultModel: "fast", Models: fastStrong},
			want: []string{"fast", "fast", "strong", "fast"},
		},
		{
			name: "prompt falls back without strong",
			cfg:  &config.Config{DefaultModel: "fast", Models: []config.Model{{Name: "fast"}}},
			want: []string{"fast", "fast", "fast", "fast"},
		},
		{
			name: "ask falls back without fast",
			cfg:  &config.Config{DefaultModel: "strong", Models: []config.Model{{Name: "strong"}}},
			want: []string{"strong", "strong", "strong", "strong"},
		},
		{
			name: "ask prefers fast over the default",
			cfg:  &config.Config{DefaultModel: "strong", Models: []config.Model{{Name: "strong"}, {Name: "fast"}}},
			want: []string{"strong", "strong", "strong", "fast"},
		},
		{
			name: "explicit model wins",
			cfg: &config.Config{
				DefaultModel: "strong",
				Models:       fastStrong,
				Modes:        []config.Mode{{ID: "prompt", Model: "fast"}},
			},
			want: []string{"fast"},
		},
		{
			name: "custom mode uses the default",
			cfg: &config.Config{
				DefaultModel: "strong",
				Models:       fastStrong,
				Modes:        []config.Mode{{ID: "summary", Name: "总结", Prompt: "总结文本"}},
			},
			want: []string{"strong"},
		},
		{
			name: "no models",
			cfg:  &config.Config{},
			want: []string{"", "", "", ""},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			modes, err := Resolve(tc.cfg)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if len(modes) != len(tc.want) {
				t.Fatalf("len(Resolve) = %d, want %d", len(modes), len(tc.want))
			}
			for i, want := range tc.want {
				if modes[i].Model != want {
					t.Errorf("modes[%d].Model = %q, want %q", i, modes[i].Model, want)
				}
			}
		})
	}
}

func TestOrganizeMergedIntoImprove(t *testing.T) {
	modes, err := Resolve(&config.Config{Modes: []config.Mode{{ID: "organize"}, {ID: "improve"}, {ID: "ask"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(modes) != 2 || modes[0].ID != "improve" || modes[1].ID != "ask" {
		t.Errorf("modes = %+v", modes)
	}
}
