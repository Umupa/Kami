package config

import (
	"errors"
	"strings"
	"testing"
)

func TestDecodeFullConfig(t *testing.T) {
	const data = `{
  "$schema": "./config.schema.json",
  "version": 1,
  "myLanguage": "日本語",
  "foreignLanguage": "English",
  "models": [
    {
      "name": "fast",
      "endpoint": "https://api.openai.com/v1/chat/completions",
      "format": "chat-completions",
      "model": "gpt-5-mini"
    },
    {
      "name": "strong",
      "endpoint": "https://api.anthropic.com/v1/messages",
      "format": "anthropic-messages",
      "model": "claude-sonnet-5-5",
      "headers": {"X-Trace-Id": "abc"},
      "body": {"max_tokens": 8192},
      "contextWindow": 200000
    }
  ],
  "modes": [
    {"id": "translate"},
    {
      "id": "summarize",
      "name": "总结",
      "prompt": "用三句话总结原文。",
      "input": "instruction",
      "output": "plain",
      "model": "strong"
    },
    {"id": "ask", "model": "fast"}
  ],
  "shortcuts": {
    "panel": "double-command",
    "selection": "control+option+a",
    "capture": "control+option+s"
  },
  "launchAtLogin": true
}`

	cfg, err := Decode([]byte(data))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.Schema != "./config.schema.json" {
		t.Errorf("Schema = %q, want %q", cfg.Schema, "./config.schema.json")
	}
	if cfg.Version != Version {
		t.Errorf("Version = %d, want %d", cfg.Version, Version)
	}
	if cfg.MyLanguage != "日本語" {
		t.Errorf("MyLanguage = %q, want 日本語", cfg.MyLanguage)
	}
	if cfg.ForeignLanguage != "English" {
		t.Errorf("ForeignLanguage = %q, want English", cfg.ForeignLanguage)
	}
	// The file leaves defaultModel out, so the first model is the default.
	if cfg.DefaultModel != "fast" {
		t.Errorf("DefaultModel = %q, want fast", cfg.DefaultModel)
	}
	if !cfg.LaunchAtLogin {
		t.Error("LaunchAtLogin = false, want true")
	}

	if len(cfg.Models) != 2 {
		t.Fatalf("len(Models) = %d, want 2", len(cfg.Models))
	}
	fast, ok := cfg.Model("fast")
	if !ok {
		t.Fatal("Model(fast) not found")
	}
	if fast.Auth != AuthBearer {
		t.Errorf("fast.Auth = %q, want %q", fast.Auth, AuthBearer)
	}
	strong, ok := cfg.Model("strong")
	if !ok {
		t.Fatal("Model(strong) not found")
	}
	if strong.Auth != AuthXAPIKey {
		t.Errorf("strong.Auth = %q, want %q", strong.Auth, AuthXAPIKey)
	}
	if strong.ContextWindow != 200000 {
		t.Errorf("strong.ContextWindow = %d, want 200000", strong.ContextWindow)
	}
	if strong.Headers["X-Trace-Id"] != "abc" {
		t.Errorf("strong.Headers = %v, want X-Trace-Id abc", strong.Headers)
	}
	if strong.Body["max_tokens"] != float64(8192) {
		t.Errorf("strong.Body[max_tokens] = %v, want 8192", strong.Body["max_tokens"])
	}

	if len(cfg.Modes) != 3 {
		t.Fatalf("len(Modes) = %d, want 3", len(cfg.Modes))
	}
	custom := cfg.Modes[1]
	if custom.ID != "summarize" || custom.Name != "总结" || custom.Prompt != "用三句话总结原文。" {
		t.Errorf("custom mode = %+v", custom)
	}
	if custom.Input != InputInstruction || custom.Output != OutputPlain || custom.Model != "strong" {
		t.Errorf("custom mode settings = %+v", custom)
	}
	wantShortcuts := Shortcuts{
		Panel:     "double-command",
		Selection: "control+option+a",
		Capture:   "control+option+s",
	}
	if cfg.Shortcuts != wantShortcuts {
		t.Errorf("Shortcuts = %+v, want %+v", cfg.Shortcuts, wantShortcuts)
	}
}

func TestDecodeMinimalConfig(t *testing.T) {
	cfg, err := Decode([]byte(`{"version":1}`))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.Version != Version {
		t.Errorf("Version = %d, want %d", cfg.Version, Version)
	}
	if len(cfg.Models) != 0 || len(cfg.Modes) != 0 {
		t.Errorf("Models = %v, Modes = %v, want both empty", cfg.Models, cfg.Modes)
	}
	if cfg.DefaultModel != "" {
		t.Errorf("DefaultModel = %q, want empty without models", cfg.DefaultModel)
	}
	if cfg.Schema != "" {
		t.Errorf("Schema = %q, want empty", cfg.Schema)
	}
}

func TestDecodeDefaults(t *testing.T) {
	const data = `{
  "version": 1,
  "models": [
    {
      "name": "fast",
      "endpoint": "https://example.com/v1/responses",
      "format": "responses",
      "model": "m"
    },
    {
      "name": "strong",
      "endpoint": "https://example.com/v1/messages",
      "format": "anthropic-messages",
      "model": "m"
    }
  ]
}`

	cfg, err := Decode([]byte(data))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if cfg.MyLanguage != DefaultMyLanguage {
		t.Errorf("MyLanguage = %q, want %q", cfg.MyLanguage, DefaultMyLanguage)
	}
	if cfg.ForeignLanguage != DefaultForeignLanguage {
		t.Errorf("ForeignLanguage = %q, want %q", cfg.ForeignLanguage, DefaultForeignLanguage)
	}
	if cfg.DefaultModel != "fast" {
		t.Errorf("DefaultModel = %q, want first model fast", cfg.DefaultModel)
	}
	if cfg.Models[0].Auth != AuthBearer {
		t.Errorf("models[0].Auth = %q, want %q", cfg.Models[0].Auth, AuthBearer)
	}
	if cfg.Models[1].Auth != AuthXAPIKey {
		t.Errorf("models[1].Auth = %q, want %q", cfg.Models[1].Auth, AuthXAPIKey)
	}
	wantShortcuts := Shortcuts{
		Panel:     DefaultPanelShortcut,
		Selection: DefaultSelectionShortcut,
		Capture:   DefaultCaptureShortcut,
	}
	if cfg.Shortcuts != wantShortcuts {
		t.Errorf("Shortcuts = %+v, want %+v", cfg.Shortcuts, wantShortcuts)
	}
}

func TestDecodeProblems(t *testing.T) {
	const fastModel = `{"name":"fast","endpoint":"https://example.com/v1","format":"chat-completions","model":"m"}`

	tests := []struct {
		name     string
		input    string
		path     string
		contains string
	}{
		{
			name: "syntax error",
			input: `{
  "version": 1
  "models": []
}`,
			path:     "",
			contains: "第 3 行",
		},
		{
			name:     "top level array",
			input:    `[{"version":1}]`,
			path:     "",
			contains: "JSON 对象",
		},
		{
			name:     "unknown top level field",
			input:    `{"version":1,"foo":true}`,
			path:     "foo",
			contains: "不认识的字段",
		},
		{
			name:     "unknown field in model",
			input:    `{"version":1,"models":[` + fastModel + `,{"name":"strong","endpoint":"https://example.com/v1","format":"responses","model":"m","foo":1}]}`,
			path:     "models[1].foo",
			contains: "不认识的字段",
		},
		{
			name:     "unknown field in shortcuts",
			input:    `{"version":1,"shortcuts":{"foo":"bar"}}`,
			path:     "shortcuts.foo",
			contains: "不认识的字段",
		},
		{
			name:     "unknown field in mode",
			input:    `{"version":1,"modes":[{"id":"translate","foo":true}]}`,
			path:     "modes[0].foo",
			contains: "不认识的字段",
		},
		{
			name:     "wrong type",
			input:    `{"version":"1"}`,
			path:     "version",
			contains: "类型",
		},
		{
			name:     "version missing",
			input:    `{"myLanguage":"English"}`,
			path:     "version",
			contains: "必须是 1",
		},
		{
			name:     "version 2",
			input:    `{"version":2}`,
			path:     "version",
			contains: "必须是 1",
		},
		{
			name:     "model empty name",
			input:    `{"version":1,"models":[{"name":"","endpoint":"https://example.com/v1","format":"chat-completions","model":"m"}]}`,
			path:     "models[0].name",
			contains: "不能为空",
		},
		{
			name:     "model duplicate name",
			input:    `{"version":1,"models":[` + fastModel + `,{"name":"fast","endpoint":"https://example.com/v1","format":"responses","model":"m"}]}`,
			path:     "models[1].name",
			contains: "重名",
		},
		{
			name:     "model endpoint scheme",
			input:    `{"version":1,"models":[{"name":"m","endpoint":"ftp://example.com/v1","format":"chat-completions","model":"m"}]}`,
			path:     "models[0].endpoint",
			contains: "http(s)",
		},
		{
			name:     "model endpoint host",
			input:    `{"version":1,"models":[{"name":"m","endpoint":"https:///v1","format":"chat-completions","model":"m"}]}`,
			path:     "models[0].endpoint",
			contains: "http(s)",
		},
		{
			name:     "model bad format",
			input:    `{"version":1,"models":[{"name":"m","endpoint":"https://example.com/v1","format":"chat","model":"m"}]}`,
			path:     "models[0].format",
			contains: "chat-completions",
		},
		{
			name:     "model empty model",
			input:    `{"version":1,"models":[{"name":"m","endpoint":"https://example.com/v1","format":"chat-completions","model":""}]}`,
			path:     "models[0].model",
			contains: "不能为空",
		},
		{
			name:     "model bad auth",
			input:    `{"version":1,"models":[{"name":"m","endpoint":"https://example.com/v1","format":"chat-completions","model":"m","auth":"basic"}]}`,
			path:     "models[0].auth",
			contains: "bearer",
		},
		{
			name:     "model invalid header name",
			input:    `{"version":1,"models":[{"name":"m","endpoint":"https://example.com/v1","format":"chat-completions","model":"m","headers":{"Bad Header":"x"}}]}`,
			path:     "models[0].headers",
			contains: "请求头",
		},
		{
			name:     "model negative context window",
			input:    `{"version":1,"models":[{"name":"m","endpoint":"https://example.com/v1","format":"chat-completions","model":"m","contextWindow":-1}]}`,
			path:     "models[0].contextWindow",
			contains: "负数",
		},
		{
			name:     "default model missing",
			input:    `{"version":1,"models":[` + fastModel + `],"defaultModel":"missing"}`,
			path:     "defaultModel",
			contains: "没有名为",
		},
		{
			name:     "mode id uppercase",
			input:    `{"version":1,"modes":[{"id":"Translate"}]}`,
			path:     "modes[0].id",
			contains: "小写字母",
		},
		{
			name:     "mode id leading dash",
			input:    `{"version":1,"modes":[{"id":"-x"}]}`,
			path:     "modes[0].id",
			contains: "小写字母",
		},
		{
			name:     "mode id underscore",
			input:    `{"version":1,"modes":[{"id":"a_b"}]}`,
			path:     "modes[0].id",
			contains: "小写字母",
		},
		{
			name:     "mode duplicate id",
			input:    `{"version":1,"modes":[{"id":"translate"},{"id":"translate"}]}`,
			path:     "modes[1].id",
			contains: "重复",
		},
		{
			name:     "mode bad input",
			input:    `{"version":1,"modes":[{"id":"translate","input":"source"}]}`,
			path:     "modes[0].input",
			contains: "material",
		},
		{
			name:     "mode bad output",
			input:    `{"version":1,"modes":[{"id":"translate","output":"html"}]}`,
			path:     "modes[0].output",
			contains: "auto",
		},
		{
			name:     "mode missing model",
			input:    `{"version":1,"modes":[{"id":"translate","model":"missing"}]}`,
			path:     "modes[0].model",
			contains: "没有名为",
		},
		{
			name:     "shortcut invalid",
			input:    `{"version":1,"shortcuts":{"panel":"control+enter"}}`,
			path:     "shortcuts.panel",
			contains: "支持",
		},
		{
			name:     "shortcuts equal",
			input:    `{"version":1,"shortcuts":{"panel":"control+option+b","selection":"control+option+b"}}`,
			path:     "shortcuts.selection",
			contains: "相同",
		},
		{
			name:     "shortcut matches another default",
			input:    `{"version":1,"shortcuts":{"selection":"double-option"}}`,
			path:     "shortcuts.selection",
			contains: "相同",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			problems := decodeProblems(t, tt.input)
			if len(problems) != 1 {
				t.Fatalf("got %d problems, want 1: %v", len(problems), problems)
			}
			p := problems[0]
			if p.Path != tt.path {
				t.Errorf("Path = %q, want %q", p.Path, tt.path)
			}
			if !strings.Contains(p.Message, tt.contains) {
				t.Errorf("Message = %q, want it to contain %q", p.Message, tt.contains)
			}
		})
	}
}

func TestDecodeHeadersAndBodyAreFreeForm(t *testing.T) {
	const data = `{
  "version": 1,
  "models": [{
    "name": "m",
    "endpoint": "https://example.com/v1",
    "format": "chat-completions",
    "model": "m",
    "headers": {"X-Trace-Id": "abc", "x-custom": "v"},
    "body": {"temperature": 0.1, "tools": null, "nested": {"a": [1, 2]}}
  }]
}`

	cfg, err := Decode([]byte(data))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := cfg.Models[0].Headers["X-Trace-Id"]; got != "abc" {
		t.Errorf("Headers[X-Trace-Id] = %q, want abc", got)
	}
	if got := cfg.Models[0].Headers["x-custom"]; got != "v" {
		t.Errorf("Headers[x-custom] = %q, want v", got)
	}
	if got := cfg.Models[0].Body["temperature"]; got != 0.1 {
		t.Errorf("Body[temperature] = %v, want 0.1", got)
	}
	if _, ok := cfg.Models[0].Body["nested"]; !ok {
		t.Error("Body[nested] missing")
	}
}

func TestDecodeReportsEveryProblem(t *testing.T) {
	const data = `{
  "version": 2,
  "models": [
    {"name": "m", "endpoint": "ftp://x", "format": "bad", "model": "", "auth": "bad"},
    {"name": "m", "endpoint": "https://example.com/v1", "format": "chat-completions", "model": "x"}
  ],
  "defaultModel": "missing",
  "modes": [{"id": "Bad"}],
  "shortcuts": {"panel": "command+control+a", "selection": "control+command+a"}
}`

	problems := decodeProblems(t, data)
	const wantCount = 9
	if len(problems) != wantCount {
		t.Fatalf("got %d problems, want %d: %v", len(problems), wantCount, problems)
	}
	want := []string{
		"version",
		"models[0].endpoint",
		"models[0].format",
		"models[0].model",
		"models[0].auth",
		"models[1].name",
		"defaultModel",
		"modes[0].id",
		"shortcuts.selection",
	}
	got := make(map[string]bool, len(problems))
	for _, p := range problems {
		got[p.Path] = true
	}
	for _, path := range want {
		if !got[path] {
			t.Errorf("missing problem at %q; problems: %v", path, problems)
		}
	}
}

func TestErrorString(t *testing.T) {
	err := &Error{Problems: []Problem{
		{Message: "文件不是 JSON 对象"},
		{Path: "version", Message: "必须是 1"},
		{Path: "models[0].name", Message: "不能为空"},
	}}
	want := "文件不是 JSON 对象\nversion：必须是 1\nmodels[0].name：不能为空"
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

// decodeProblems requires Decode to fail with an *Error and returns its
// problems.
func decodeProblems(t *testing.T, data string) []Problem {
	t.Helper()
	cfg, err := Decode([]byte(data))
	if err == nil {
		t.Fatal("Decode succeeded, want an error")
	}
	if cfg != nil {
		t.Fatalf("Decode returned config %+v with error, want nil", cfg)
	}
	var ce *Error
	if !errors.As(err, &ce) {
		t.Fatalf("error is %T, want *Error", err)
	}
	return ce.Problems
}
