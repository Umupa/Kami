package panel

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestDarkSnapshot(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"## 标题\n\n正文 `code`\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n```go\nfunc main() {}\n```\n"}})
	h.tt.SetDark(true)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("hello")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.snapshot("dark")
}
