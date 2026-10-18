package config

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// PromptInput is what the configuration prompt describes.
type PromptInput struct {
	// Exe is the app's executable, which is also its command line.
	Exe string
	// Dir holds config.json and its schema.
	Dir      string
	Snapshot Snapshot
	// HasKey reports whether a model's API key is in the keychain.
	HasKey func(model string) bool
}

// AgentPrompt returns the prompt the user hands an AI assistant to configure
// the app (Cida's configuration.md §五): where everything is, how keys stay
// out of the conversation, how to check, and the task that fits the current
// state. It never contains a key.
func AgentPrompt(in PromptInput) string {
	exe := quote(in.Exe)
	cfgPath := filepath.Join(in.Dir, FileName)
	schemaPath := filepath.Join(in.Dir, SchemaFileName)
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	w("请帮我配置 cida-next 使用的大模型服务。cida-next 是一个 macOS 菜单栏应用，你需要能在这台 Mac 上运行命令。\n\n")
	w("## 怎么配置\n\n")
	w("- 设置都在 %s，每个字段的含义、取值和示例在 %s：先读 schema，再直接编辑 config.json。保存后正在运行的应用立即生效；文件不合法时，应用继续用上一份合法配置。\n", cfgPath, schemaPath)
	w("- API Key 不写进 config.json，也不要出现在对话里、命令行参数里或任何文件里。需要时请我把 Key 复制到剪贴板，然后运行：\n\n  pbpaste | %s key set <模型名> --stdin\n\n  Key 已经在环境变量或文件里时，改用 --env <变量名> 或 --file <路径>。已经保存过的 Key 不用再要。\n", exe)
	w("- 改完运行 %s check 检查全部模型。失败时运行 %s check <模型名> --verbose，看实际发出的请求和服务的回应，按这些证据修改后再检查；不要重复同一个失败的做法。\n", exe, exe)
	w("- 模型的推理（thinking、reasoning）能关就关，关不掉就设到最低档：翻译、润色用不到它，只会变慢。用模型的 body 字段设置，以服务商文档为准。\n")
	w("- 只改需要改的字段，不要擅自更换我选的服务或模型。需要我提供信息、授权或处理账户问题时，说明要我做什么，然后等我。\n\n")

	w("## 现在的配置\n\n以下摘要生成于复制时，可能已过时，以 config.json 为准。\n\n")
	c := in.Snapshot.Config
	if len(c.Models) == 0 {
		w("- 还没有模型。\n")
	}
	for _, m := range c.Models {
		key := "Key 未设置"
		switch {
		case m.Auth == AuthNone:
			key = "不需要 Key"
		case in.HasKey != nil && in.HasKey(m.Name):
			key = "Key 已保存"
		}
		w("- %s：%s，%s，%s，%s\n", m.Name, m.Format, m.Model, host(m.Endpoint), key)
	}
	if in.Snapshot.Err != nil {
		w("- config.json 现在有问题，应用在用上一份合法配置：\n")
		for _, line := range strings.Split(in.Snapshot.Err.Error(), "\n") {
			w("  - %s\n", line)
		}
	}

	w("\n## 任务\n\n")
	switch {
	case in.Snapshot.Err != nil:
		w("先读 config.json 和上面的问题，把它修好，再检查。\n")
	case len(c.Models) == 0:
		w("问我用哪家服务和哪个模型，然后从头配置。建议配两个模型：一个快而便宜的叫 fast，用于问答；一个能力强的叫 strong，用于整理和提示词梳理。只配一个也可以。\n")
	default:
		w("问我想调整什么，默认保留现有的服务和模型。\n")
	}
	w("\n最后运行 %s check 确认，简短告诉我是否可用，以及我还需要做什么。\n", exe)
	return b.String()
}

// quote makes a path safe to paste into a shell.
func quote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

func host(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		return u.Host
	}
	return endpoint
}
