package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentPromptQuotesPaths(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "Application Support", "Kami")
	cases := []struct {
		name       string
		exe        string
		wantQuoted string
	}{
		{"space", "/Applications/My Apps/Kami", `'/Applications/My Apps/Kami'`},
		{"single quote", "/Applications/O'Brien/Kami", `'/Applications/O'\''Brien/Kami'`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prompt := AgentPrompt(PromptInput{Exe: tc.exe, Dir: dir, Config: &Config{Version: Version}})
			for _, want := range []string{
				"Kami 的命令行：" + tc.wantQuoted + "\n",
				"配置文件：'" + filepath.Join(dir, FileName) + "'\n",
				"字段说明：'" + filepath.Join(dir, SchemaFileName) + "'\n",
				"`Kami check`",
				"`Kami check --verbose`",
				"model.apiKey",
			} {
				if !strings.Contains(prompt, want) {
					t.Errorf("prompt does not contain %q:\n%s", want, prompt)
				}
			}
		})
	}
}

// decodeFor decodes a config for a prompt test, defaults filled in.
func decodeFor(t *testing.T, data string) *Config {
	t.Helper()
	c, err := Decode([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// The prompt does not depend on the state of the configuration: the agent
// learns it from `Kami check`, so a copy taken earlier stays right.
func TestAgentPromptIsTheSameInEveryState(t *testing.T) {
	const model = `"model":{"endpoint":"https://api.example.com/v1/chat/completions","format":"chat-completions","model":"m"`
	want := AgentPrompt(PromptInput{Exe: "/bin/Kami", Dir: "/tmp/Kami", Config: decodeFor(t, `{"version":1}`)})
	for name, data := range map[string]string{
		"no key":  `{"version":1,` + model + `}}`,
		"working": `{"version":1,` + model + `,"apiKey":"k"}}`,
	} {
		if got := AgentPrompt(PromptInput{Exe: "/bin/Kami", Dir: "/tmp/Kami", Config: decodeFor(t, data)}); got != want {
			t.Errorf("%s: the prompt differs from the one with no model:\n%s", name, got)
		}
	}
	for _, step := range []string{
		"运行 `Kami check`，按结果走",
		"- 还没有配置模型 → 带我从头配置",
		"- 配置文件有问题 → 直接修好",
		"- 请求失败 → 运行 `Kami check --verbose`",
		"- 已经可用 → 告诉我一声，问我想调整什么",
		"一次只问一个问题，给编号选项",
		"2. 没有 Key，但有订阅",
		"用 magpie（https://github.com/yetone/magpie）",
		"http://127.0.0.1:3425",
		"我同意后再安装",
		"你用 pbpaste 读出来，直接写进 model.apiKey",
		"## 选模型",
		"优先选便宜、快、不带思考的模型，比如 DeepSeek、GPT 的 mini / nano 系列、`gpt6-luna`",
		"Claude Haiku 这类模型单价高出不少，除非我点名要，不要默认选它",
	} {
		if !strings.Contains(want, step) {
			t.Errorf("prompt does not say %q:\n%s", step, want)
		}
	}
}

func TestAgentPromptSendsUserBack(t *testing.T) {
	for _, tc := range []struct{ shortcuts, want string }{
		{``, "回到 Kami，按 ⌥Space 打开面板，输入一段文字按 ⏎ 试一次。"},
		{`,"shortcuts":{"panel":"none"}`, "回到 Kami，从菜单栏打开面板，输入一段文字按 ⏎ 试一次。"},
	} {
		prompt := AgentPrompt(PromptInput{Exe: "/bin/Kami", Dir: t.TempDir(), Config: decodeFor(t, `{"version":1`+tc.shortcuts+`}`)})
		if !strings.Contains(prompt, tc.want) {
			t.Errorf("prompt does not say %q:\n%s", tc.want, prompt)
		}
	}
}

func TestAgentPromptDoesNotLeakConfigValues(t *testing.T) {
	cfg := &Config{
		Schema:          "secret-schema-value",
		Version:         Version,
		MyLanguage:      "secret-language-value",
		ForeignLanguage: "secret-foreign-value",
		Model: &Model{
			Endpoint:      "https://api.example.com/secret-path?token=secret-query",
			Format:        FormatChatCompletions,
			Model:         "alpha-model",
			APIKey:        "secret-api-key-value",
			Auth:          AuthBearer,
			Headers:       map[string]string{"X-Secret-Header": "secret-header-value"},
			Body:          map[string]any{"api_key": "secret-body-value"},
			ContextWindow: 424242,
		},
		Shortcuts: Shortcuts{Panel: "secret-shortcut-value"},
	}
	prompt := AgentPrompt(PromptInput{Exe: "/bin/Kami", Dir: t.TempDir(), Config: cfg})
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
		"alpha-model",
		"api.example.com",
	} {
		if strings.Contains(prompt, secret) {
			t.Errorf("prompt leaks the config value %q", secret)
		}
	}
}
