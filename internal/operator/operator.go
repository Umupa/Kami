// Package operator is the panel's operator mode: a source that starts with
// /operator asks about the app's own settings or changes them, in natural
// language, answered by the configured model. It is the only way to see and
// change settings in the app; model services are left to a coding agent.
package operator

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/Umupa/Gloss/internal/config"
)

// Command is what starts operator mode at the start of a source.
const Command = "/operator"

var leading = regexp.MustCompile(`^\s*/operator(\s|$)`)

// Leading reports whether text starts with the command, and returns the
// request after it.
func Leading(text string) (request string, ok bool) {
	loc := leading.FindStringIndex(text)
	if loc == nil {
		return text, false
	}
	return strings.TrimSpace(text[loc[1]:]), true
}

// Token returns the runes of the command in text, for styling it: start and
// end rune offsets, ok false when text does not start with it.
func Token(text string) (start, end int, ok bool) {
	i := strings.Index(text, Command)
	if i < 0 || strings.TrimSpace(text[:i]) != "" {
		return 0, 0, false
	}
	rest := text[i+len(Command):]
	if rest != "" && !strings.ContainsAny(rest[:1], " \t\n") {
		return 0, 0, false
	}
	start = utf8.RuneCountInString(text[:i])
	return start, start + utf8.RuneCountInString(Command), true
}

// fence opens the block at the end of a reply that carries what to change.
const fence = "```gloss-operator"

// Plan is what a reply asks the app to do.
type Plan struct {
	Ops     []config.Op `json:"ops"`
	Actions []string    `json:"actions"`
}

// Actions the model may ask for.
const (
	ActionCopyAgentPrompt = "copy-agent-prompt"
	ActionOpenPermission  = "open-permission:" // + accessibility, input-monitoring, screen-recording
)

// Visible returns the part of a reply to show: without the plan block, and,
// while the reply streams, without a line that may be the block's start.
func Visible(reply string) string {
	if i := strings.Index(reply, fence); i >= 0 {
		return strings.TrimRight(reply[:i], " \t\n")
	}
	last := reply[strings.LastIndex(reply, "\n")+1:]
	if last != "" && strings.HasPrefix(fence, last) && strings.HasPrefix(last, "`") {
		return strings.TrimRight(reply[:len(reply)-len(last)], " \t\n")
	}
	return reply
}

// ParsePlan reads the plan block of a finished reply; ok is false when it
// has none.
func ParsePlan(reply string) (p Plan, ok bool, err error) {
	i := strings.Index(reply, fence)
	if i < 0 {
		return Plan{}, false, nil
	}
	body := reply[i+len(fence):]
	if j := strings.Index(body, "```"); j >= 0 {
		body = body[:j]
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &p); err != nil {
		return Plan{}, true, fmt.Errorf("operator: unreadable plan: %w", err)
	}
	return p, true, nil
}

// ErrNoModel is an operator request without a model to answer it.
var ErrNoModel = errors.New("operator: no model configured")

// ChangesText says what an edit changed, for the end of the reply.
func ChangesText(changes []config.Change) string {
	if len(changes) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\n---\n\n已修改（⌘Z 撤销）：\n\n")
	for _, c := range changes {
		before, after := shown(c.Before), shown(c.After)
		if strings.HasPrefix(c.Path, "shortcuts.") {
			before, after = shortcut(c.Before), shortcut(c.After)
		}
		fmt.Fprintf(&b, "- `%s`：%s → %s\n", c.Path, before, after)
	}
	return b.String()
}

// shortcut shows a shortcut value as its glyphs, as ⌃⌥S.
func shortcut(v string) string {
	var s string
	if json.Unmarshal([]byte(v), &s) != nil || s == "" {
		return shown(v)
	}
	sc, err := config.ParseShortcut(s)
	if err != nil {
		return shown(v)
	}
	return sc.Glyphs()
}

func shown(v string) string {
	if v == "" {
		return "（无）"
	}
	const max = 80
	if utf8.RuneCountInString(v) > max {
		v = string([]rune(v)[:max]) + "…"
	}
	return "`" + strings.ReplaceAll(v, "`", "'") + "`"
}
