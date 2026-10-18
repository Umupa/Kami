package panel

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/llm"
	"github.com/Umupa/cida-next/internal/operator"
	"github.com/Umupa/cida-next/internal/session"
)

// operatorHarness records what operator mode asks of the app.
type operatorHarness struct {
	*harness
	applied [][]config.Op
	undone  int
	actions []string
	system  string
}

func newOperatorHarness(t *testing.T, reply string) *operatorHarness {
	oh := &operatorHarness{}
	client := clientFunc(func(ctx context.Context, req llm.Request, onText func(string)) error {
		oh.system = req.System
		for _, piece := range strings.SplitAfter(reply, "\n") {
			onText(piece)
		}
		return nil
	})
	oh.harness = newHarness(t, client)
	oh.p.deps.Operator = Operator{
		State: func() operator.State { return operator.State{Version: "0.1.0", Config: oh.p.cfg} },
		Apply: func(ops []config.Op) ([]config.Change, error) {
			oh.applied = append(oh.applied, ops)
			return []config.Change{{Path: "shortcuts.capture", Before: `"control+option+s"`, After: `"option+s"`}}, nil
		},
		Undo: func() bool { oh.undone++; return true },
		Run:  func(a string) { oh.actions = append(oh.actions, a) },
	}
	return oh
}

func TestOperatorReplacesModeTabs(t *testing.T) {
	oh := newOperatorHarness(t, "")
	oh.tt.Frame()
	oh.tt.Click("原文")
	oh.tt.Type("/operator")
	oh.tt.Frame()
	if !oh.tt.HasText("Operator") || oh.tt.HasText("润色") {
		t.Errorf("texts %q: want the Operator pill in place of the mode tabs", oh.tt.Texts())
	}
	oh.snapshot("operator")
	if err := oh.tt.Click("退出 Operator"); err != nil {
		t.Fatal(err)
	}
	if oh.p.operatorActive() || !oh.tt.HasText("润色") {
		t.Errorf("× did not leave operator mode; source %q", oh.p.source)
	}
}

func TestOperatorAloneShowsGuide(t *testing.T) {
	oh := newOperatorHarness(t, "never asked")
	oh.tt.Frame()
	oh.tt.Click("原文")
	oh.tt.Type("/operator")
	oh.tt.Key(0, ui.KeyEnter)
	oh.drain()
	turn := oh.p.sess.Last()
	if !turn.Local || !strings.Contains(turn.Reply, "## Operator") || oh.system != "" {
		t.Errorf("local %v, asked the model %v, reply %q", turn.Local, oh.system != "", turn.Reply)
	}
}

func TestOperatorAppliesPlanAndUndoes(t *testing.T) {
	reply := "好的，截图识别改成 ⌥S。\n```cida-operator\n{\"ops\":[{\"path\":\"shortcuts.capture\",\"value\":\"option+s\"}],\"actions\":[\"copy-agent-prompt\"]}\n```\n"
	oh := newOperatorHarness(t, reply)
	oh.tt.Frame()
	oh.tt.Click("原文")
	oh.tt.Type("/operator 截图识别改成 ⌥S")
	oh.tt.Key(0, ui.KeyEnter)
	oh.drain()

	turn := oh.p.sess.Last()
	if !strings.Contains(oh.system, "operator of cida-next") {
		t.Error("the request did not carry the operator prompt")
	}
	if oh.p.sess.Turns[0].Ask != "截图识别改成 ⌥S" {
		t.Errorf("asked %q, want the request without /operator", oh.p.sess.Turns[0].Ask)
	}
	if len(oh.applied) != 1 || oh.applied[0][0].Path != "shortcuts.capture" || len(oh.actions) != 1 {
		t.Fatalf("applied %v, actions %v", oh.applied, oh.actions)
	}
	if strings.Contains(turn.Reply, "cida-operator") || !strings.Contains(turn.Reply, "已修改") || !turn.Undoable {
		t.Errorf("reply %q, undoable %v", turn.Reply, turn.Undoable)
	}
	oh.tt.Advance(time.Second)
	oh.snapshotNow("operator-sparkles")
	oh.tt.Advance(3 * time.Second) // past the sparkles
	oh.snapshot("operator-changed")

	oh.tt.Click("追问")
	oh.tt.Key(ui.Cmd, ui.KeyZ)
	if oh.undone != 1 || turn.Undoable {
		t.Errorf("⌘Z in the empty follow-up: undone %d, still undoable %v", oh.undone, turn.Undoable)
	}
}

func TestOperatorFollowUpsStayInOperator(t *testing.T) {
	oh := newOperatorHarness(t, "现在的快捷键：…")
	oh.tt.Frame()
	oh.tt.Click("原文")
	oh.tt.Type("/operator 快捷键是什么")
	oh.tt.Key(0, ui.KeyEnter)
	oh.drain()
	oh.system = ""
	oh.tt.Click("追问")
	oh.tt.Type("那模型呢")
	oh.tt.Key(0, ui.KeyEnter)
	oh.drain()
	if !strings.Contains(oh.system, "operator of cida-next") || oh.p.sess.Last().Status != session.Done {
		t.Error("a follow-up left operator mode")
	}
	if len(oh.applied) != 0 {
		t.Error("a question changed settings")
	}
}

func TestOperatorBlockHiddenWhileStreaming(t *testing.T) {
	for _, in := range []string{"改好了。\n``", "改好了。\n```cida-op", "改好了。\n```cida-operator\n{\"ops\":"} {
		if got := operator.Visible(in); got != "改好了。" {
			t.Errorf("Visible(%q) = %q", in, got)
		}
	}
}
