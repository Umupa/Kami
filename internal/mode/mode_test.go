package mode

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Umupa/Kami/internal/config"
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
	}
}

func TestResolveBuiltinOverrides(t *testing.T) {
	base := Mode{
		ID:     "improve",
		Name:   "润色",
		Desc:   "改得通顺准确；内容杂乱时整理成标题、列表和表格",
		Prompt: improvePrompt,
		Input:  config.InputMaterial,
		Output: config.OutputAuto,
	}
	cases := []struct {
		name  string
		entry config.Mode
		want  Mode
	}{
		{
			name:  "name",
			entry: config.Mode{ID: "improve", Name: "自定义名字"},
			want: func() Mode {
				m := base
				m.Name = "自定义名字"
				return m
			}(),
		},
		{
			name:  "prompt",
			entry: config.Mode{ID: "improve", Prompt: "自定义提示词"},
			want: func() Mode {
				m := base
				m.Prompt = "自定义提示词"
				return m
			}(),
		},
		{
			name:  "input",
			entry: config.Mode{ID: "improve", Input: config.InputInstruction},
			want: func() Mode {
				m := base
				m.Input = config.InputInstruction
				return m
			}(),
		},
		{
			name:  "output",
			entry: config.Mode{ID: "improve", Output: config.OutputPlain},
			want: func() Mode {
				m := base
				m.Output = config.OutputPlain
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
		Modes: []config.Mode{{ID: "summary", Name: "总结", Prompt: "总结文本"}},
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
		{ID: "improve"},
	}}
	modes, err := Resolve(cfg)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	wantIDs := []string{"ask", "translate", "improve"}
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

func TestOrganizeMergedIntoImprove(t *testing.T) {
	modes, err := Resolve(&config.Config{Modes: []config.Mode{{ID: "organize"}, {ID: "improve"}, {ID: "ask"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(modes) != 2 || modes[0].ID != "improve" || modes[1].ID != "ask" {
		t.Errorf("modes = %+v", modes)
	}
}

func TestResolveRemovedPrompt(t *testing.T) {
	modes, err := Resolve(&config.Config{Modes: []config.Mode{{ID: "translate"}, {ID: "prompt"}, {ID: "ask"}}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	var ids []string
	for _, m := range modes {
		ids = append(ids, m.ID)
	}
	if want := []string{"translate", "ask"}; !slices.Equal(ids, want) {
		t.Errorf("modes = %v, want %v: the gone built-in left out", ids, want)
	}

	modes, err = Resolve(&config.Config{Modes: []config.Mode{{ID: "prompt", Name: "梳理", Prompt: "整理成提示词"}}})
	if err != nil {
		t.Fatalf("Resolve with its own prompt: %v", err)
	}
	if len(modes) != 1 || modes[0].Name != "梳理" || modes[0].Prompt != "整理成提示词" {
		t.Errorf("modes = %+v, want a custom mode with its own name and prompt", modes)
	}
}
