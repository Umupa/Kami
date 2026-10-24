//go:build soak

package panel

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/Umupa/Kami/internal/soak"
	"github.com/egoist/mygo/ui"
)

func TestSoakPanel(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"## 回复\n\n", "| 项目 | 值 |\n|---|---|\n| 状态 | 正常 |\n\n", "```go\nfmt.Println(42)\n```\n"}})
	rounds := soak.Rounds(t, 500)
	var baseline, final soak.Sample
	for i := 1; i <= rounds; i++ {
		h.p.importCaptured(fmt.Sprintf("Generated source %d", i))
		h.drain()
		h.tt.Key(0, ui.KeyTab)
		h.p.run()
		h.drain()
		if h.p.sess == nil || h.p.sess.Last().Reply == "" {
			t.Fatal("missing reply")
		}
		h.p.reset()
		h.tt.Advance(2 * time.Second)
		h.tt.Frame()
		// The test recorder itself is not application state.
		h.sizes = nil
		if i == rounds/5 || i%max(1, rounds/5) == 0 || i == rounds {
			final = soak.Measure(t, i)
			if i == rounds/5 {
				baseline = final
			}
		}
	}
	soak.Check(t, baseline, final)
	soak.Idle(t)
	runtime.KeepAlive(h)
}
