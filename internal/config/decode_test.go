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
  "model": {
    "endpoint": "https://api.anthropic.com/v1/messages",
    "format": "anthropic-messages",
    "model": "claude-sonnet-5-5",
    "headers": {"X-Trace-Id": "abc"},
    "body": {"max_tokens": 8192},
    "contextWindow": 200000
  },
  "modes": [
    {"id": "translate"},
    {
      "id": "summarize",
      "name": "总结",
      "prompt": "用三句话总结原文。",
      "input": "instruction",
      "output": "plain"
    },
    {"id": "ask"}
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
	if !cfg.LaunchAtLogin {
		t.Error("LaunchAtLogin = false, want true")
	}

	strong := cfg.Model
	if strong == nil {
		t.Fatal("Model is nil")
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
	if custom.Input != InputInstruction || custom.Output != OutputPlain {
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

func TestDecodeAPIKeyRoundTrip(t *testing.T) {
	const key = "sk-round-trip-distinctive"
	const data = `{
  "version": 1,
  "model": {
    "endpoint": "https://example.com/v1",
    "format": "chat-completions",
    "model": "m",
    "apiKey": "sk-round-trip-distinctive"
  }
}`

	cfg, err := Decode([]byte(data))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := cfg.Model.APIKey; got != key {
		t.Fatalf("APIKey = %q, want %q", got, key)
	}
	encoded, err := Encode(cfg)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.Contains(string(encoded), `"apiKey": "`+key+`"`) {
		t.Errorf("Encode did not write apiKey:\n%s", encoded)
	}
	again, err := Decode(encoded)
	if err != nil {
		t.Fatalf("Decode(Encode): %v", err)
	}
	if got := again.Model.APIKey; got != key {
		t.Errorf("decoded APIKey = %q, want %q", got, key)
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
	if cfg.Model != nil || len(cfg.Modes) != 0 {
		t.Errorf("Model = %v, Modes = %v, want neither", cfg.Model, cfg.Modes)
	}
	if cfg.Schema != "" {
		t.Errorf("Schema = %q, want empty", cfg.Schema)
	}
}

func TestDecodeDefaults(t *testing.T) {
	const data = `{
  "version": 1,
  "model": {
    "endpoint": "https://example.com/v1/responses",
    "format": "responses",
    "model": "m"
  }
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
	if cfg.Model.Auth != AuthBearer {
		t.Errorf("model.Auth = %q, want %q", cfg.Model.Auth, AuthBearer)
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
  "modes": []
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
			contains: "未知字段",
		},
		{
			name:     "unknown field in model",
			input:    `{"version":1,"model":{"endpoint":"https://example.com/v1","format":"responses","model":"m","foo":1}}`,
			path:     "model.foo",
			contains: "未知字段",
		},
		{
			name:     "unknown field in shortcuts",
			input:    `{"version":1,"shortcuts":{"foo":"bar"}}`,
			path:     "shortcuts.foo",
			contains: "未知字段",
		},
		{
			name:     "unknown field in mode",
			input:    `{"version":1,"modes":[{"id":"translate","foo":true}]}`,
			path:     "modes[0].foo",
			contains: "未知字段",
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
			name:     "models of an earlier shape",
			input:    `{"version":1,"models":[]}`,
			path:     "models",
			contains: "未知字段",
		},
		{
			name:     "model endpoint scheme",
			input:    `{"version":1,"model":{"endpoint":"ftp://example.com/v1","format":"chat-completions","model":"m"}}`,
			path:     "model.endpoint",
			contains: "http(s)",
		},
		{
			name:     "model endpoint host",
			input:    `{"version":1,"model":{"endpoint":"https:///v1","format":"chat-completions","model":"m"}}`,
			path:     "model.endpoint",
			contains: "http(s)",
		},
		{
			name:     "model bad format",
			input:    `{"version":1,"model":{"endpoint":"https://example.com/v1","format":"chat","model":"m"}}`,
			path:     "model.format",
			contains: "chat-completions",
		},
		{
			name:     "model empty model",
			input:    `{"version":1,"model":{"endpoint":"https://example.com/v1","format":"chat-completions","model":""}}`,
			path:     "model.model",
			contains: "不能为空",
		},
		{
			name:     "model bad auth",
			input:    `{"version":1,"model":{"endpoint":"https://example.com/v1","format":"chat-completions","model":"m","auth":"basic"}}`,
			path:     "model.auth",
			contains: "bearer",
		},
		{
			name:     "model invalid header name",
			input:    `{"version":1,"model":{"endpoint":"https://example.com/v1","format":"chat-completions","model":"m","headers":{"Bad Header":"x"}}}`,
			path:     "model.headers",
			contains: "请求头",
		},
		{
			name:     "model negative context window",
			input:    `{"version":1,"model":{"endpoint":"https://example.com/v1","format":"chat-completions","model":"m","contextWindow":-1}}`,
			path:     "model.contextWindow",
			contains: "负数",
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
			name:     "mode with a model of its own",
			input:    `{"version":1,"modes":[{"id":"translate","model":"fast"}]}`,
			path:     "modes[0].model",
			contains: "未知字段",
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
			input:    `{"version":1,"shortcuts":{"selection":"option+space"}}`,
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
  "model": {
    "endpoint": "https://example.com/v1",
    "format": "chat-completions",
    "model": "m",
    "headers": {"X-Trace-Id": "abc", "x-custom": "v"},
    "body": {"temperature": 0.1, "tools": null, "nested": {"a": [1, 2]}}
  }
}`

	cfg, err := Decode([]byte(data))
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got := cfg.Model.Headers["X-Trace-Id"]; got != "abc" {
		t.Errorf("Headers[X-Trace-Id] = %q, want abc", got)
	}
	if got := cfg.Model.Headers["x-custom"]; got != "v" {
		t.Errorf("Headers[x-custom] = %q, want v", got)
	}
	if got := cfg.Model.Body["temperature"]; got != 0.1 {
		t.Errorf("Body[temperature] = %v, want 0.1", got)
	}
	if _, ok := cfg.Model.Body["nested"]; !ok {
		t.Error("Body[nested] missing")
	}
}

func TestDecodeReportsEveryProblem(t *testing.T) {
	const data = `{
  "version": 2,
  "model": {"endpoint": "ftp://x", "format": "bad", "model": "", "auth": "bad"},
  "modes": [{"id": "Bad"}],
  "shortcuts": {"panel": "command+control+a", "selection": "control+command+a"}
}`

	problems := decodeProblems(t, data)
	const wantCount = 7
	if len(problems) != wantCount {
		t.Fatalf("got %d problems, want %d: %v", len(problems), wantCount, problems)
	}
	want := []string{
		"version",
		"model.endpoint",
		"model.format",
		"model.model",
		"model.auth",
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
		{Path: "model.model", Message: "不能为空"},
	}}
	want := "文件不是 JSON 对象\nversion：必须是 1\nmodel.model：不能为空"
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
