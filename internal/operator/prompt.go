package operator

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/design"
	"github.com/Umupa/Kami/internal/mode"
)

// State is what the operator knows about the app.
type State struct {
	// Palettes holds every built-in and custom palette by theme name.
	Palettes map[string]map[string]string
	// Dark is the system appearance, which auto follows.
	Dark    bool
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
	w(`You are the operator of Kami, a macOS menu bar app: a floating panel that sends text to a language model in a mode (a prompt preset) and shows the answer as Markdown. The user talks to you to see and change the app's settings. Answer in the user's language, briefly, in Markdown. Put anything the user may want to copy, such as a path, a shortcut value, a prompt or a piece of config.json, in a fenced code block: code blocks have a copy button. Use tables for lists of settings.

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
	w("%s", themeSection(s))
	w("\nPermissions:\n")
	w("(macOS ties a grant to the app's signature. A switch shown on in System Settings while the app reports it not granted belongs to an earlier build: the user removes Kami from that list with −, then adds it again. Input Monitoring and Screen Recording take effect after a restart: 重新启动 Kami in the menu bar menu.)\n")
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
	b.WriteString("## Operator\n\n用一句话查看或修改 Kami 的设置，改动立即生效，⌘Z 撤销。可以试试：\n\n")
	for _, ex := range []string{
		"现在的快捷键是什么",
		"截图识别改成 ⌥S",
		"翻译模式用的提示词是什么",
		"加一个把改动写成 commit message 的模式",
		"问答默认用 strong 模型",
		"把强调色改成橙色",
		"基于 gruvbox 做一个更暗的版本",
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

func themeSection(s State) string {
	var b strings.Builder
	b.WriteString(`
## Custom themes

Custom themes live in config.json "themes", an array. A theme resolves its built-in base plus its colors overrides. Unless explicitly set, border/muted/faint/accentText follow background and text; edge follows accent; operatorFill follows operator.

Never edit built-in themes. To change one, append a custom theme at themes[N] where N is the current array length, or write the whole array. Use that built-in as base and include only changed colors. Use the user's id, or <base>-custom, adding -2, -3 if taken. Set theme to the new id in the same ops block. If the theme in effect is already custom, edit its colors in place.

Keep text against background at 4.5:1 or more, muted at 3:1; when changing accent, keep accentText readable on it. Colors are #RRGGBB. After changing, say in one sentence what changed and that ⌘Z undoes it.

| name | where |
| --- | --- |
| background | 面板背景 |
| surface | 代码块、表头、标签的底色 |
| raised | 选中行、当前标签 |
| border | 分隔线、区块边框 |
| edge | 面板外框 |
| text | 正文 |
| bright | 加粗文字、原文 |
| muted | 说明、元信息 |
| faint | 占位符、按键提示 |
| accent | 执行按钮、标题、当前模式 |
| accentText | 强调色上的文字 |
| link | 链接 |
| code | 行内代码 |
| quote | 引用竖线 |
| operator | Operator 的命令、标签与星星 |
| operatorFill | Operator 标签底色 |
| sparkle | 修改生效时的星星 |
| danger | 错误、删除 |
| keyword / string / number / comment / function / type | 代码高亮 |

`)
	name := config.DefaultTheme
	if s.Config != nil {
		name = s.Config.Theme
	}
	if name == "" {
		name = config.DefaultTheme
	}
	configured := name
	if name == "auto" {
		name = design.AutoLight
		if s.Dark {
			name = design.AutoDark
		}
	}
	fmt.Fprintf(&b, "Theme in effect: %s (configured: %s).\n\n", name, configured)
	for _, key := range config.ThemeColors {
		fmt.Fprintf(&b, "- %s: %s\n", key, s.Palettes[name][key])
	}
	b.WriteString("\nBuilt-in themes (name, appearance, background, text, accent):\n")
	for _, name := range design.ThemeNames {
		appearance := "light"
		if design.Themes[name].Dark {
			appearance = "dark"
		}
		colors := s.Palettes[name]
		fmt.Fprintf(&b, "- %s, %s, %s, %s, %s\n", name, appearance, colors["background"], colors["text"], colors["accent"])
	}
	return b.String()
}
