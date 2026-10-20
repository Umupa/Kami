package llm

import (
	"context"
	"math/rand/v2"
	"strings"
	"time"
)

// Demo streams a canned reply in uneven bursts, as services do, for
// development builds without a configured model.
type Demo struct{}

func (Demo) Stream(ctx context.Context, req Request, onText func(string)) error {
	reply := demoReply
	if n := len(req.Messages); n > 1 {
		reply = "好的，按你的要求调整：\n\n> " + strings.ReplaceAll(req.Messages[n-1].Content, "\n", "\n> ") + "\n\n这是演示回复：配置一个模型后，这里会是真正的回答。"
	}
	for len(reply) > 0 {
		n := min(len(reply), 4+rand.IntN(40))
		for n < len(reply) && !utf8Start(reply[n]) {
			n++
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(20+rand.IntN(120)) * time.Millisecond):
		}
		onText(reply[:n])
		reply = reply[n:]
	}
	return nil
}

func utf8Start(b byte) bool { return b&0xC0 != 0x80 }

const demoReply = "## 演示回复\n\n" +
	"还没有配置模型，这是一段**演示用**的流式回复，用来检查 Markdown 渲染和出字手感。\n\n" +
	"| 事项 | 负责人 | 截止 |\n|------|--------|------|\n| 面板唤起 | 我 | 周五 |\n| 流式渲染 | MyGo | 已完成 |\n| 宽表格里很长很长的一段说明文字，用来检查换行 | — | — |\n\n" +
	"代码块：\n\n```go\nfunc main() {\n\tfmt.Println(\"你好，Gloss\")\n}\n```\n\n" +
	"- 列表第一项，包含 `行内代码`\n- 第二项，[一个链接](https://example.com)\n  - 嵌套项\n- [x] 已完成的任务\n- [ ] 未完成的任务\n\n" +
	"> 引用：配置提示词交给 Claude Code，它会帮你配好模型。\n\n" +
	"最后一段英文混排 mixed with English words, to check the line height and the spacing between 中文 and Latin text."
