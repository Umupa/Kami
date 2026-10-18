package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
)

const opsTestData = `{
  "version": 1,
  "myLanguage": "简体中文",
  "models": [
    {
      "name": "fast",
      "endpoint": "https://example.com/v1/chat/completions",
      "format": "chat-completions",
      "model": "m"
    }
  ],
  "modes": [
    {"id": "translate"},
    {"id": "summarize", "name": "总结", "prompt": "old prompt"}
  ],
  "shortcuts": {
    "panel": "double-option",
    "selection": "control+option+a",
    "capture": "control+option+s"
  }
}`

func TestApplyOps(t *testing.T) {
	const noShortcuts = `{"version":1,"modes":[{"id":"translate"}]}`

	tests := []struct {
		name    string
		data    string
		ops     []Op
		changes []Change
		wantOut []string
		wantNot []string
	}{
		{
			name: "set scalar",
			ops:  []Op{{Path: "myLanguage", Value: json.RawMessage(`"日本語"`)}},
			changes: []Change{{
				Path: "myLanguage", Before: `"简体中文"`, After: `"日本語"`,
			}},
			wantOut: []string{`"myLanguage": "日本語"`},
		},
		{
			name: "set inside missing shortcuts object",
			data: noShortcuts,
			ops:  []Op{{Path: "shortcuts.capture", Value: json.RawMessage(`"control+option+c"`)}},
			changes: []Change{{
				Path: "shortcuts.capture", Before: "", After: `"control+option+c"`,
			}},
			wantOut: []string{`"shortcuts": {`, `"capture": "control+option+c"`},
			wantNot: []string{`"panel"`, `"selection"`},
		},
		{
			name: "replace mode entry",
			ops: []Op{{
				Path:  "modes[1]",
				Value: json.RawMessage(`{"id":"summarize","name":"总结","prompt":"new prompt"}`),
			}},
			changes: []Change{{
				Path:   "modes[1]",
				Before: `{"id":"summarize","name":"总结","prompt":"old prompt"}`,
				After:  `{"id":"summarize","name":"总结","prompt":"new prompt"}`,
			}},
			wantOut: []string{`"prompt": "new prompt"`},
		},
		{
			name: "append mode entry",
			ops: []Op{{
				Path:  "modes[2]",
				Value: json.RawMessage(`{"prompt":"做点什么","name":"自定义","output":"plain","id":"custom"}`),
			}},
			changes: []Change{{
				Path:   "modes[2]",
				Before: "",
				After:  `{"id":"custom","name":"自定义","output":"plain","prompt":"做点什么"}`,
			}},
			wantOut: []string{`"id": "custom"`, `"name": "自定义"`, `"output": "plain"`},
		},
		{
			name: "set mode prompt",
			ops:  []Op{{Path: "modes[1].prompt", Value: json.RawMessage(`"new prompt"`)}},
			changes: []Change{{
				Path: "modes[1].prompt", Before: `"old prompt"`, After: `"new prompt"`,
			}},
			wantOut: []string{`"prompt": "new prompt"`},
		},
		{
			name: "set boolean",
			ops:  []Op{{Path: "launchAtLogin", Value: json.RawMessage(`true`)}},
			changes: []Change{{
				Path: "launchAtLogin", Before: "", After: "true",
			}},
			wantOut: []string{`"launchAtLogin": true`},
		},
		{
			name: "remove key",
			ops:  []Op{{Path: "myLanguage", Value: json.RawMessage(`null`)}},
			changes: []Change{{
				Path: "myLanguage", Before: `"简体中文"`, After: "",
			}},
			wantNot: []string{`"myLanguage"`},
		},
		{
			name: "remove array element shifts later elements",
			ops:  []Op{{Path: "modes[0]", Value: json.RawMessage(`null`)}},
			changes: []Change{{
				Path:   "modes[0]",
				Before: `{"id":"translate"}`,
				After:  `{"id":"summarize","name":"总结","prompt":"old prompt"}`,
			}},
			wantNot: []string{`"translate"`},
		},
		{
			name: "no-op edits",
			ops: []Op{
				{Path: "myLanguage", Value: json.RawMessage(`"简体中文"`)},
				{Path: "foreignLanguage", Value: json.RawMessage(`null`)},
			},
			changes: nil,
		},
		{
			name: "several ops on one path report one change",
			ops: []Op{
				{Path: "myLanguage", Value: json.RawMessage(`"A"`)},
				{Path: "myLanguage", Value: json.RawMessage(`"B"`)},
			},
			changes: []Change{{
				Path: "myLanguage", Before: `"简体中文"`, After: `"B"`,
			}},
			wantOut: []string{`"myLanguage": "B"`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.data
			if data == "" {
				data = opsTestData
			}
			input := []byte(data)
			original := bytes.Clone(input)

			out, changes, err := ApplyOps(input, tt.ops, nil)
			if err != nil {
				t.Fatalf("ApplyOps: %v", err)
			}
			if !bytes.Equal(input, original) {
				t.Error("ApplyOps changed its input data")
			}
			if !slices.Equal(changes, tt.changes) {
				t.Errorf("changes = %+v, want %+v", changes, tt.changes)
			}
			if !json.Valid(out) {
				t.Errorf("output is not valid JSON:\n%s", out)
			}
			if !bytes.HasSuffix(out, []byte("\n")) {
				t.Error("output does not end with a newline")
			}
			if _, err := Decode(out); err != nil {
				t.Errorf("Decode(output): %v\n%s", err, out)
			}
			for _, want := range tt.wantOut {
				if !bytes.Contains(out, []byte(want)) {
					t.Errorf("output does not contain %q:\n%s", want, out)
				}
			}
			for _, not := range tt.wantNot {
				if bytes.Contains(out, []byte(not)) {
					t.Errorf("output contains %q:\n%s", not, out)
				}
			}
		})
	}
}

func TestApplyOpsErrors(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		ops      []Op
		path     string
		contains string
	}{
		{
			name:     "models refused",
			ops:      []Op{{Path: "models", Value: json.RawMessage(`[]`)}},
			path:     "models",
			contains: "agent",
		},
		{
			name:     "model key refused",
			ops:      []Op{{Path: "models[0].apiKey", Value: json.RawMessage(`"x"`)}},
			path:     "models[0].apiKey",
			contains: "models",
		},
		{
			name:     "version refused",
			ops:      []Op{{Path: "version", Value: json.RawMessage(`2`)}},
			path:     "version",
			contains: "version",
		},
		{
			name:     "schema refused",
			ops:      []Op{{Path: "$schema", Value: json.RawMessage(`"x"`)}},
			path:     "$schema",
			contains: "$schema",
		},
		{
			name:     "empty path",
			ops:      []Op{{Path: "", Value: json.RawMessage(`"x"`)}},
			path:     "",
			contains: "路径",
		},
		{
			name:     "trailing dot",
			ops:      []Op{{Path: "modes.", Value: json.RawMessage(`"x"`)}},
			path:     "modes.",
			contains: "字段名",
		},
		{
			name:     "leading dot",
			ops:      []Op{{Path: ".modes", Value: json.RawMessage(`"x"`)}},
			path:     ".modes",
			contains: "字段名",
		},
		{
			name:     "index is not a number",
			ops:      []Op{{Path: "modes[x]", Value: json.RawMessage(`"x"`)}},
			path:     "modes[x]",
			contains: "下标",
		},
		{
			name:     "unclosed index",
			ops:      []Op{{Path: "modes[0", Value: json.RawMessage(`"x"`)}},
			path:     "modes[0",
			contains: "下标",
		},
		{
			name:     "empty index",
			ops:      []Op{{Path: "modes[]", Value: json.RawMessage(`"x"`)}},
			path:     "modes[]",
			contains: "下标",
		},
		{
			name:     "trailing junk after index",
			ops:      []Op{{Path: "modes[0]x", Value: json.RawMessage(`"x"`)}},
			path:     "modes[0]x",
			contains: "应为 . 或 [",
		},
		{
			name:     "hyphen in key",
			ops:      []Op{{Path: "my-language", Value: json.RawMessage(`"x"`)}},
			path:     "my-language",
			contains: "应为 . 或 [",
		},
		{
			name:     "double dot",
			ops:      []Op{{Path: "modes..prompt", Value: json.RawMessage(`"x"`)}},
			path:     "modes..prompt",
			contains: "字段名",
		},
		{
			name:     "index too large",
			ops:      []Op{{Path: "modes[5]", Value: json.RawMessage(`{"id":"x","name":"x","prompt":"x"}`)}},
			path:     "modes[5]",
			contains: "超出范围",
		},
		{
			name:     "path through a string",
			ops:      []Op{{Path: "myLanguage.extra", Value: json.RawMessage(`1`)}},
			path:     "myLanguage.extra",
			contains: "不是对象",
		},
		{
			name:     "index through a string",
			ops:      []Op{{Path: "shortcuts.panel[0]", Value: json.RawMessage(`"x"`)}},
			path:     "shortcuts.panel[0]",
			contains: "不是数组",
		},
		{
			name:     "invalid JSON value",
			ops:      []Op{{Path: "myLanguage", Value: json.RawMessage(`not-json`)}},
			path:     "myLanguage",
			contains: "合法的 JSON",
		},
		{
			name:     "empty JSON value",
			ops:      []Op{{Path: "shortcuts.panel", Value: nil}},
			path:     "shortcuts.panel",
			contains: "合法的 JSON",
		},
		{
			name:     "result fails Decode with a bad shortcut",
			ops:      []Op{{Path: "shortcuts.panel", Value: json.RawMessage(`"control+enter"`)}},
			path:     "shortcuts.panel",
			contains: "支持",
		},
		{
			name:     "result fails Decode with a bad mode id",
			ops:      []Op{{Path: "modes[1].id", Value: json.RawMessage(`"Bad"`)}},
			path:     "modes[1].id",
			contains: "小写",
		},
		{
			name:     "result fails Decode with a wrong type",
			ops:      []Op{{Path: "launchAtLogin", Value: json.RawMessage(`"yes"`)}},
			path:     "launchAtLogin",
			contains: "类型",
		},
		{
			name:     "result fails Decode with an unknown field",
			ops:      []Op{{Path: "shortcuts.foo", Value: json.RawMessage(`"x"`)}},
			path:     "shortcuts.foo",
			contains: "不认识的字段",
		},
		{
			name:     "input is not JSON",
			data:     `{`,
			path:     "",
			contains: "JSON",
		},
		{
			name:     "input is not an object",
			data:     `[]`,
			path:     "",
			contains: "JSON 对象",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.data
			if data == "" {
				data = opsTestData
			}
			input := []byte(data)
			original := bytes.Clone(input)

			out, changes, err := ApplyOps(input, tt.ops, nil)
			if err == nil {
				t.Fatalf("ApplyOps succeeded, want an error; output:\n%s", out)
			}
			if out != nil {
				t.Errorf("output = %q, want nil", out)
			}
			if changes != nil {
				t.Errorf("changes = %+v, want nil", changes)
			}
			if !bytes.Equal(input, original) {
				t.Error("ApplyOps changed its input data")
			}

			var ce *Error
			if !errors.As(err, &ce) {
				t.Fatalf("error is %T, want *Error: %v", err, err)
			}
			found := false
			for _, p := range ce.Problems {
				if p.Path != tt.path {
					continue
				}
				found = true
				if !strings.Contains(p.Message, tt.contains) {
					t.Errorf("problem message = %q, want it to contain %q", p.Message, tt.contains)
				}
			}
			if !found {
				t.Errorf("no problem at path %q in %+v", tt.path, ce.Problems)
			}
		})
	}
}

func TestApplyOpsCheckFailure(t *testing.T) {
	const data = `{"version":1,"modes":[{"id":"translate"}]}`
	input := []byte(data)
	original := bytes.Clone(input)

	sentinel := errors.New("custom modes are incomplete")
	called := false
	check := func(c *Config) error {
		called = true
		// check sees the same defaults-filled Config that Store gives it.
		if c.Shortcuts.Panel != DefaultPanelShortcut {
			t.Errorf("check got Panel = %q, want %q", c.Shortcuts.Panel, DefaultPanelShortcut)
		}
		return sentinel
	}

	out, changes, err := ApplyOps(input, []Op{{
		Path:  "myLanguage",
		Value: json.RawMessage(`"English"`),
	}}, check)
	if err == nil {
		t.Fatalf("ApplyOps succeeded, want check failure; output:\n%s", out)
	}
	if !called {
		t.Error("check was not called")
	}
	if out != nil || changes != nil {
		t.Errorf("out = %q, changes = %+v, want both nil", out, changes)
	}
	if !bytes.Equal(input, original) {
		t.Error("ApplyOps changed its input data")
	}
	var ce *Error
	if !errors.As(err, &ce) {
		t.Fatalf("error is %T, want *Error: %v", err, err)
	}
	if len(ce.Problems) != 1 || ce.Problems[0].Path != "" {
		t.Errorf("problems = %+v, want one at the file root", ce.Problems)
	}
	if !strings.Contains(ce.Problems[0].Message, sentinel.Error()) {
		t.Errorf("message = %q, want it to contain %q", ce.Problems[0].Message, sentinel.Error())
	}
}

func TestApplyOpsKeepsAPIKey(t *testing.T) {
	const key = "sk-ops-keep-distinctive"
	const data = `{
  "version": 1,
  "models": [
    {
      "name": "fast",
      "endpoint": "https://example.com/v1/chat/completions",
      "format": "chat-completions",
      "model": "m",
      "apiKey": "sk-ops-keep-distinctive"
    }
  ]
}`

	out, changes, err := ApplyOps([]byte(data), []Op{{
		Path:  "myLanguage",
		Value: json.RawMessage(`"English"`),
	}}, nil)
	if err != nil {
		t.Fatalf("ApplyOps: %v", err)
	}
	if !bytes.Contains(out, []byte(`"apiKey": "`+key+`"`)) {
		t.Errorf("output did not preserve the apiKey byte for byte:\n%s", out)
	}
	if !bytes.Contains(out, []byte(`"myLanguage": "English"`)) {
		t.Errorf("output did not apply the edit:\n%s", out)
	}
	// Defaults are not written into the file.
	for _, field := range []string{`"shortcuts"`, `"foreignLanguage"`, `"defaultModel"`} {
		if bytes.Contains(out, []byte(field)) {
			t.Errorf("output contains default field %q:\n%s", field, out)
		}
	}
	for _, change := range changes {
		for _, text := range []string{change.Path, change.Before, change.After} {
			if strings.Contains(text, key) {
				t.Errorf("change %+v contains the API key", change)
			}
		}
	}
	if len(changes) != 1 || changes[0].Path != "myLanguage" {
		t.Errorf("changes = %+v, want only myLanguage", changes)
	}
}
