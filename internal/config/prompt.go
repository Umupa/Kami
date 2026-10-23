package config

import (
	"fmt"
	"path/filepath"
	"strings"
)

// PromptInput is what the configuration prompt points the agent at.
type PromptInput struct {
	// Exe is the app's executable, which is also its command line.
	Exe string
	// Dir holds config.json and its schema.
	Dir    string
	Config *Config
}

// AgentPrompt returns the prompt the user hands a coding agent to set up,
// fix or adjust the model. It is the same in every state: the agent runs
// `Kami check` to learn the state, so a copy never goes stale. It walks the
// user through choosing a service one question at a time, and keeps the key
// out of the conversation. It never contains a key or any configured value.
func AgentPrompt(in PromptInput) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	w("你好！请帮我把 Kami 连上一个 AI 模型。Kami 是我 Mac 菜单栏里的翻译、润色和问答小工具，只需要一个又快又便宜的模型。\n\n")
	w("Kami 的命令行：%s\n", quote(in.Exe))
	w("（下文的 `Kami` 都指它，请用带引号的绝对路径运行）\n")
	w("配置文件：%s\n", quote(filepath.Join(in.Dir, FileName)))
	w("字段说明：%s\n\n", quote(filepath.Join(in.Dir, SchemaFileName)))
	w("和我说话时请用大白话，一次只问一个问题，给编号选项让我回数字。需要我动手时，告诉我点哪里，然后等我说「好了」。\n\n")

	w("## 先看看现在的情况（不用问我）\n\n")
	w("读字段说明和配置文件，再运行 `Kami check`，按结果走：\n")
	w("- 还没有配置模型 → 带我从头配置（见下一节）\n")
	w("- 配置文件有问题 → 直接修好，不用问我\n")
	w("- 缺 API Key → 按下面「规矩」里的办法请我复制\n")
	w("- 请求失败 → 运行 `Kami check --verbose` 看实际的请求和回应，找到原因就修；需要我换服务、充值或重新登录时再问我\n")
	w("- 已经可用 → 告诉我一声，问我想调整什么\n\n")

	w("## 从头配置\n\n")
	w("先自己看一眼：有没有装 magpie、127.0.0.1:3425 有没有在运行、有没有 Ollama 或 LM Studio 在运行。然后问我手上有哪种，结合你看到的情况推荐一个：\n\n")
	w("1. 某家服务的 API Key（OpenAI、Anthropic、DeepSeek、Kimi、智谱……）\n")
	w("2. 没有 Key，但有订阅：Claude、ChatGPT / Codex、GitHub Copilot、Gemini 等\n")
	w("3. 本机跑的模型（Ollama、LM Studio）\n")
	w("4. 都没有，或者不确定\n\n")
	w("你现在运行所用的订阅或 Key，多半就能拿来用，可以提醒我这一点。\n\n")
	w("- 选 1：问是哪家，查它的官方文档，确定接口，选这家最便宜、最快的那档模型。\n")
	w("- 选 2：用 magpie（https://github.com/yetone/magpie）在本机把订阅变成接口。先用一句话说明它是什么，我同意后再安装。在 magpie 里登录订阅由我自己来，你告诉我怎么做。然后把 Kami 指向 http://127.0.0.1:3425，按下面「选模型」挑一个。顺便提醒我三件事：Kami 用的是这份订阅的额度；用 Kami 时 magpie 要开着；有些服务商不允许把订阅用在别的工具里，由我自己判断。\n")
	w("- 选 3：指向本机的接口，auth 设为 none。\n")
	w("- 选 4：从 DeepSeek、OpenAI 的 GPT mini / nano、`gpt6-luna` 这类低价模型里推荐两三个，每个一句话说清价格和差别，让我选。\n\n")

	w("## 选模型\n\n")
	w("Kami 的请求多而短，价格和速度比能力重要。优先选便宜、快、不带思考的模型，比如 DeepSeek、GPT 的 mini / nano 系列、`gpt6-luna`。Claude Haiku 这类模型单价高出不少，除非我点名要，不要默认选它；订阅里能选的模型也按这个顺序挑。\n\n")

	w("## 规矩\n\n")
	w("- 只改配置文件里的 model，别的设置不动；我选好的服务和模型不擅自更换。保存后 Kami 立即生效，不用重启。\n")
	w("- Kami 只做翻译、润色和简短问答，用不上思考：能关就在 model.body 里关掉，关不掉就设到最低档。\n")
	w("- API Key 别让我发给你，也别打印出来或写进别的文件。需要时请我先复制，你用 pbpaste 读出来，直接写进 model.apiKey。已有的 Key 不用重新要。\n")
	w("- 每次改完运行 `Kami check`。同一个做法失败了就换思路，别反复重试。\n\n")

	w("## 最后\n\n")
	w("用一两句话告诉我结果：能不能用、还要我做什么，不讲技术细节。能用的话告诉我：回到 Kami，%s，输入一段文字按 ⏎ 试一次。\n\n", openPanel(in.Config))
	w("如果你没法在这台 Mac 上运行命令，请直接告诉我：需要一个能在本机运行命令的 Coding Agent。\n")
	return b.String()
}

// openPanel says how the user brings up the panel.
func openPanel(c *Config) string {
	if c == nil {
		return "从菜单栏打开面板"
	}
	if sc, err := ParseShortcut(c.Shortcuts.Panel); err == nil && !sc.IsZero() {
		return "按 " + sc.Glyphs() + " 打开面板"
	}
	return "从菜单栏打开面板"
}

// quote makes a path safe to paste into a shell.
func quote(path string) string {
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
