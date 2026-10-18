package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentPromptQuotesExecutable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config dir")
	cases := []struct {
		name       string
		exe        string
		wantQuoted string
	}{
		{
			name:       "space",
			exe:        "/Applications/My Apps/cida-next",
			wantQuoted: `'/Applications/My Apps/cida-next'`,
		},
		{
			name:       "single quote",
			exe:        "/Applications/O'Brien/cida-next",
			wantQuoted: `'/Applications/O'\''Brien/cida-next'`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prompt := AgentPrompt(PromptInput{
				Exe:      tc.exe,
				Dir:      dir,
				Snapshot: Snapshot{Config: &Config{Version: Version}},
			})
			for _, want := range []string{
				filepath.Join(dir, FileName),
				filepath.Join(dir, SchemaFileName),
				"apiKey",
				tc.wantQuoted + " check 检查全部模型",
				tc.wantQuoted + " check <模型名> --verbose",
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("prompt does not contain %q", want)
				}
			}
			if strings.Contains(prompt, "key set") {
				t.Error("prompt still tells the assistant to run key set")
			}
		})
	}
}

func TestAgentPromptModelSummary(t *testing.T) {
	const fastKey = "sk-fast-distinctive-secret"
	cfg := &Config{Models: []Model{
		{
			Name:     "fast",
			Endpoint: "https://fast.example.com/v1/chat/completions",
			Format:   FormatChatCompletions,
			Model:    "gpt-fast",
			APIKey:   fastKey,
			Auth:     AuthBearer,
		},
		{
			Name:     "strong",
			Endpoint: "https://api.strong.example.com:8443/v1/messages",
			Format:   FormatAnthropicMessages,
			Model:    "claude",
			Auth:     AuthNone,
		},
		{
			Name:     "local",
			Endpoint: "http://127.0.0.1:8080/v1",
			Format:   FormatResponses,
			Model:    "local",
			Auth:     AuthBearer,
		},
	}}
	prompt := AgentPrompt(PromptInput{
		Exe:      "/bin/cida-next",
		Dir:      t.TempDir(),
		Snapshot: Snapshot{Config: cfg},
	})

	wantLines := []string{
		"- fast：chat-completions，gpt-fast，fast.example.com，Key 已配置\n",
		"- strong：anthropic-messages，claude，api.strong.example.com:8443，不需要 Key\n",
		"- local：responses，local，127.0.0.1:8080，Key 未设置\n",
	}
	for _, want := range wantLines {
		if got := strings.Count(prompt, want); got != 1 {
			t.Errorf("prompt contains the summary line %q %d times, want 1", want, got)
		}
	}
	if strings.Contains(prompt, fastKey) {
		t.Error("prompt contains the model's API key")
	}
}

func TestAgentPromptTask(t *testing.T) {
	t.Run("no models", func(t *testing.T) {
		prompt := AgentPrompt(PromptInput{
			Exe:      "/bin/cida-next",
			Dir:      t.TempDir(),
			Snapshot: Snapshot{Config: &Config{Version: Version}},
		})
		task := promptTask(t, prompt)
		for _, want := range []string{"问我用哪家服务", "fast", "strong"} {
			if !strings.Contains(task, want) {
				t.Errorf("task does not mention %q", want)
			}
		}
	})

	t.Run("invalid config", func(t *testing.T) {
		err := &Error{Problems: []Problem{
			{Path: "models[0].endpoint", Message: "必须是完整的 http(s) 地址"},
			{Path: "modes[0].name", Message: "自定义模式必须写名字"},
		}}
		prompt := AgentPrompt(PromptInput{
			Exe:      "/bin/cida-next",
			Dir:      t.TempDir(),
			Snapshot: Snapshot{Config: &Config{Version: Version}, Err: err},
		})
		task := promptTask(t, prompt)
		if !strings.Contains(task, "先读 config.json 和上面的问题，把它修好，再检查。") {
			t.Errorf("task does not say to fix the file: %q", task)
		}
		if strings.Contains(task, "问我用哪家服务") {
			t.Error("task asks for a service while the file needs fixing")
		}
		for _, line := range []string{
			"  - models[0].endpoint：必须是完整的 http(s) 地址\n",
			"  - modes[0].name：自定义模式必须写名字\n",
		} {
			if got := strings.Count(prompt, line); got != 1 {
				t.Errorf("prompt contains problem line %q %d times, want 1", line, got)
			}
		}
	})

	t.Run("models", func(t *testing.T) {
		cfg := &Config{
			Version: Version,
			Models:  []Model{{Name: "fast", Format: FormatChatCompletions, Model: "gpt-fast"}},
		}
		prompt := AgentPrompt(PromptInput{
			Exe:      "/bin/cida-next",
			Dir:      t.TempDir(),
			Snapshot: Snapshot{Config: cfg},
		})
		task := promptTask(t, prompt)
		if !strings.Contains(task, "问我想调整什么，默认保留现有的服务和模型。") {
			t.Errorf("task does not ask what to change: %q", task)
		}
		if strings.Contains(task, "问我用哪家服务") {
			t.Error("task asks for a service although models are configured")
		}
	})
}

func promptTask(t *testing.T, prompt string) string {
	t.Helper()
	const marker = "## 任务\n\n"
	i := strings.Index(prompt, marker)
	if i < 0 {
		t.Fatalf("prompt has no task section: %q", prompt)
	}
	return prompt[i+len(marker):]
}

func TestAgentPromptDoesNotLeakConfigValues(t *testing.T) {
	cfg := &Config{
		Schema:          "secret-schema-value",
		Version:         Version,
		MyLanguage:      "secret-language-value",
		ForeignLanguage: "secret-foreign-value",
		DefaultModel:    "alpha",
		Models: []Model{{
			Name:          "alpha",
			Endpoint:      "https://api.example.com/secret-path?token=secret-query",
			Format:        FormatChatCompletions,
			Model:         "alpha-model",
			APIKey:        "secret-api-key-value",
			Auth:          AuthBearer,
			Headers:       map[string]string{"X-Secret-Header": "secret-header-value"},
			Body:          map[string]any{"api_key": "secret-body-value"},
			ContextWindow: 424242,
		}},
		Shortcuts: Shortcuts{Panel: "secret-shortcut-value"},
	}
	prompt := AgentPrompt(PromptInput{
		Exe:      "/bin/cida-next",
		Dir:      t.TempDir(),
		Snapshot: Snapshot{Config: cfg},
	})

	for _, secret := range []string{
		"secret-schema-value",
		"secret-language-value",
		"secret-foreign-value",
		"secret-api-key-value",
		"secret-path",
		"secret-query",
		"secret-header-value",
		"secret-body-value",
		"424242",
		"secret-shortcut-value",
	} {
		if strings.Contains(prompt, secret) {
			t.Errorf("prompt leaks the config value %q", secret)
		}
	}
	wantLine := "- alpha：chat-completions，alpha-model，api.example.com，Key 已配置\n"
	if got := strings.Count(prompt, wantLine); got != 1 {
		t.Errorf("prompt contains the summary line %q %d times, want 1", wantLine, got)
	}
}
