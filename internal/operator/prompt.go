package operator

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/mode"
)

// State is what the operator knows about the app.
type State struct {
	Version string
	// ConfigPath is config.json's path; File its contents as written.
	ConfigPath string
	File       []byte
	// Config is the configuration in effect, defaults filled in.
	Config      *config.Config
	Permissions []Permission
	// AgentPrompt is the prompt that has a coding agent configure model
	// services.
	AgentPrompt string
}

// Permission is a system permission and whether it is granted.
type Permission struct {
	ID, Name, Purpose string
	Granted           bool
}

// System returns the system prompt of an operator turn. API keys never
// appear in it.
func System(s State) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }
	w(`You are the operator of cida-next, a macOS menu bar app: a floating panel that sends text to a language model in a mode (a prompt preset) and shows the answer as Markdown. The user talks to you to see and change the app's settings. Answer in the user's language, briefly, in Markdown.

Settings live in config.json, described by the JSON Schema below. You may change anything in it except model services: never edit "models", "version" or "$schema". When the user wants to add, change or remove a model service, an endpoint or an API key, explain that a coding agent configures those, and ask for the action %q, which copies the prompt for it.

To change settings, end your reply with one block, after your explanation:

%s
{"ops": [{"path": "shortcuts.capture", "value": "option+s"}], "actions": []}
`+"```"+`

Each op sets the value at a path of config.json as written (dotted keys and [index]); a null value removes it. When "modes" is absent from the file, the built-in modes apply; to change one of them, write the whole "modes" array, listing every mode to keep in order, each built-in one as {"id": "..."} plus the fields to override. Shortcut values use the schema's syntax. Actions: %q; %q followed by accessibility, input-monitoring or screen-recording, which opens that page of System Settings. Write the block only to change something or run an action; never for a question. The app applies the block, checks it, and appends what changed; if it is invalid nothing changes and the user sees why.

`, ActionCopyAgentPrompt, fence, ActionCopyAgentPrompt, ActionOpenPermission)

	w("## The app now\n\nVersion %s. config.json: %s\n\n", s.Version, s.ConfigPath)
	w("config.json as written, API keys removed:\n\n```json\n%s\n```\n\n", masked(s.File))
	if c := s.Config; c != nil {
		w("In effect, with defaults: my language %s; foreign language %s; default model %q; launch at login %v.\n\n", c.MyLanguage, c.ForeignLanguage, c.DefaultModel, c.LaunchAtLogin)
		w("Shortcuts: panel %s, bring in the selection %s, screenshot %s.\n\n", c.Shortcuts.Panel, c.Shortcuts.Selection, c.Shortcuts.Capture)
		w("Model services:\n")
		if len(c.Models) == 0 {
			w("- none yet\n")
		}
		for _, m := range c.Models {
			key := "no API key"
			if m.APIKey != "" {
				key = "API key set"
			}
			w("- %s: %s, %s at %s, %s\n", m.Name, m.Format, m.Model, host(m.Endpoint), key)
		}
		if modes, err := mode.Resolve(c); err == nil {
			w("\nModes in order (the first is the default), with the prompt each uses:\n")
			for _, m := range modes {
				w("\n### %s (id %s, input %s, output %s, model %q)\n\n%s\n", m.Name, m.ID, m.Input, m.Output, m.Model, m.Prompt)
			}
		}
	}
	w("\nPermissions:\n")
	for _, p := range s.Permissions {
		w("- %s (%s): %s\n", p.Name, p.Purpose, map[bool]string{true: "granted", false: "not granted"}[p.Granted])
	}
	w("\n## config.json schema\n\n```json\n%s\n```\n", config.Schema())
	return b.String()
}

// masked returns config.json without its API keys.
func masked(file []byte) string {
	var doc map[string]any
	if err := json.Unmarshal(file, &doc); err != nil {
		return "(config.json is not valid JSON right now)"
	}
	if models, ok := doc["models"].([]any); ok {
		for _, m := range models {
			if obj, ok := m.(map[string]any); ok {
				if _, has := obj["apiKey"]; has {
					obj["apiKey"] = "(hidden)"
				}
			}
		}
	}
	out, _ := json.MarshalIndent(doc, "", "  ")
	return string(out)
}

func host(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		return u.Host
	}
	return endpoint
}

// Guide is what /operator alone shows, without asking a model: what the mode
// does, and, before any model is configured, how to configure one.
func Guide(s State) string {
	var b strings.Builder
	b.WriteString("## Operator\n\n用一句话查看或修改 cida-next 的设置，改动立即生效，⌘Z 撤销。可以试试：\n\n")
	for _, ex := range []string{
		"现在的快捷键是什么",
		"截图识别改成 ⌥S",
		"翻译模式用的提示词是什么",
		"加一个把改动写成 commit message 的模式",
		"问答默认用 strong 模型",
		"登录时自动打开",
		"哪些权限还没给",
	} {
		b.WriteString("- `/operator " + ex + "`\n")
	}
	if s.Config != nil && len(s.Config.Models) > 0 {
		b.WriteString("\n模型服务（地址、API Key）交给 Coding Agent 配置：说「/operator 配置模型」，我会把提示词复制给你。\n")
		return b.String()
	}
	b.WriteString("\n还没有配置模型，Operator 需要一个模型来理解你的话。把这段提示词交给 Claude Code 等 Coding Agent，它会帮你配好：\n\n")
	b.WriteString("```text\n" + s.AgentPrompt + "\n```\n")
	return b.String()
}
