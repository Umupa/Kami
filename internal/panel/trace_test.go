package panel

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/Umupa/Kami/internal/session"
)

func TestLayoutTraceIsOptInAndOmitsContent(t *testing.T) {
	for _, enabled := range []string{"", "1"} {
		t.Run("enabled="+enabled, func(t *testing.T) {
			t.Setenv("KAMI_TRACE_LAYOUT", enabled)
			h := newHarness(t, script{})
			var buf bytes.Buffer
			h.p.deps.Log = slog.New(slog.NewTextHandler(&buf, nil))
			h.p.source = "private-source-marker"
			h.p.sess = &session.Session{Turns: []session.Turn{{Reply: "private-reply-marker", Status: session.Streaming}}}
			h.tt.Frame()
			got := buf.String()
			if enabled == "" {
				if got != "" {
					t.Fatal("layout diagnostics logged without opt-in")
				}
				return
			}
			for _, field := range []string{"event=paint", "event=turn", "event=resize-request", "frame_ns=", "scroll_y="} {
				if !strings.Contains(got, field) {
					t.Errorf("missing diagnostic field %q", field)
				}
			}
			for _, secret := range []string{h.p.source, h.p.sess.Last().Reply, testAPIKey} {
				if strings.Contains(got, secret) {
					t.Fatal("layout diagnostics included private content")
				}
			}
		})
	}
}
