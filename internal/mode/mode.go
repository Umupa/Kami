// Package mode turns the mode list of config.json into the modes the panel
// offers, and builds the system prompt of a session. A mode is a prompt
// preset: every mode goes through the same pipeline.
package mode

import (
	"fmt"
	"strings"

	"github.com/Umupa/Kami/internal/config"
)

// Mode is a resolved mode, every field filled in.
type Mode struct {
	ID   string
	Name string
	// Desc says in a line what the mode does, for the slash menu: the
	// built-in modes' own, a custom mode's first line of its prompt.
	Desc   string
	Prompt string
	Input  config.Input
	Output config.Output
}

var builtins = []Mode{
	{ID: "translate", Name: "翻译", Desc: "外文译成我的语言，我的语言译成外语", Input: config.InputMaterial, Output: config.OutputAuto, Prompt: translatePrompt},
	{ID: "improve", Name: "润色", Desc: "改得通顺准确；内容杂乱时整理成标题、列表和表格", Input: config.InputMaterial, Output: config.OutputAuto, Prompt: improvePrompt},
	{ID: "prompt", Name: "提示词", Desc: "把模糊的需求梳理成结构化的提示词", Input: config.InputMaterial, Output: config.OutputMarkdown, Prompt: promptPrompt},
	{ID: "ask", Name: "问答", Desc: "直接回答问题，先给结论", Input: config.InputInstruction, Output: config.OutputMarkdown, Prompt: askPrompt},
}

func findBuiltin(id string) (Mode, bool) {
	for _, b := range builtins {
		if b.ID == id {
			return b, true
		}
	}
	return Mode{}, false
}

// Resolve returns the modes of c in order; the first is the default mode.
// Without a mode list, they are the built-in modes. A failure is a
// *config.Error naming the entries that are incomplete.
func Resolve(c *config.Config) ([]Mode, error) {
	entries := c.Modes
	if len(entries) == 0 {
		for _, b := range builtins {
			entries = append(entries, config.Mode{ID: b.ID})
		}
	}
	var problems []config.Problem
	modes := make([]Mode, 0, len(entries))
	seen := map[string]bool{}
	for i, e := range entries {
		if e.ID == "organize" {
			// Organizing was merged into improving.
			e.ID = "improve"
		}
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		m, isBuiltin := findBuiltin(e.ID)
		if !isBuiltin {
			m = Mode{ID: e.ID, Input: config.InputMaterial, Output: config.OutputMarkdown}
			path := fmt.Sprintf("modes[%d]", i)
			if e.Name == "" {
				problems = append(problems, config.Problem{Path: path + ".name", Message: "自定义模式必须写名字"})
			}
			if e.Prompt == "" {
				problems = append(problems, config.Problem{Path: path + ".prompt", Message: "自定义模式必须写提示词"})
			}
		}
		if e.Name != "" {
			m.Name = e.Name
		}
		if e.Prompt != "" {
			m.Prompt = e.Prompt
			if !isBuiltin {
				m.Desc = firstLine(e.Prompt)
			}
		}
		if e.Input != "" {
			m.Input = e.Input
		}
		if e.Output != "" {
			m.Output = e.Output
		}
		modes = append(modes, m)
	}
	if len(problems) > 0 {
		return nil, &config.Error{Problems: problems}
	}
	return modes, nil
}

// Check validates the mode list of c, for config.NewStore.
func Check(c *config.Config) error {
	_, err := Resolve(c)
	return err
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}
