package panel

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/llm"
	"github.com/Umupa/Kami/internal/operator"
	"github.com/Umupa/Kami/internal/session"
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
	reply := "好的，截图识别改成 ⌥S。\n```kami-operator\n{\"ops\":[{\"path\":\"shortcuts.capture\",\"value\":\"option+s\"}],\"actions\":[\"copy-agent-prompt\"]}\n```\n"
	oh := newOperatorHarness(t, reply)
	oh.tt.Frame()
	oh.tt.Click("原文")
	oh.tt.Type("/operator 截图识别改成 ⌥S")
	oh.tt.Key(0, ui.KeyEnter)
	oh.drain()

	turn := oh.p.sess.Last()
	if !strings.Contains(oh.system, "operator of Kami") {
		t.Error("the request did not carry the operator prompt")
	}
	if oh.p.sess.Turns[0].Ask != "截图识别改成 ⌥S" {
		t.Errorf("asked %q, want the request without /operator", oh.p.sess.Turns[0].Ask)
	}
	if len(oh.applied) != 1 || oh.applied[0][0].Path != "shortcuts.capture" || len(oh.actions) != 1 {
		t.Fatalf("applied %v, actions %v", oh.applied, oh.actions)
	}
	if strings.Contains(turn.Reply, "kami-operator") || !strings.Contains(turn.Reply, "已修改") || !turn.Undoable {
		t.Errorf("reply %q, undoable %v", turn.Reply, turn.Undoable)
	}
	if oh.tt.HasText("执行") {
		t.Error("the mode bar still shows during the operator session")
	}
	if !oh.tt.Focused("追问") {
		t.Error("the keyboard stayed in the source after the operator request")
	}
	oh.tt.Advance(time.Second)
	oh.snapshotNow("operator-sparkles")
	oh.tt.Advance(3 * time.Second) // past the sparkles
	oh.snapshot("operator-changed")
	oh.p.source = "截图识别改成 ⌥S" // leaving operator mode brings the bar back
	oh.tt.Frame()
	if !oh.tt.HasText("执行") {
		t.Error("no mode bar after leaving operator mode")
	}
	oh.p.source = "/operator 截图识别改成 ⌥S"
	oh.tt.Frame()

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
	if !strings.Contains(oh.system, "operator of Kami") || oh.p.sess.Last().Status != session.Done {
		t.Error("a follow-up left operator mode")
	}
	if len(oh.applied) != 0 {
		t.Error("a question changed settings")
	}
}

func TestOperatorBlockHiddenWhileStreaming(t *testing.T) {
	for _, in := range []string{"改好了。\n``", "改好了。\n```kami-op", "改好了。\n```kami-operator\n{\"ops\":"} {
		if got := operator.Visible(in); got != "改好了。" {
			t.Errorf("Visible(%q) = %q", in, got)
		}
	}
}

func TestOperatorCustomThemeContrast(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		warn       bool
	}{
		{"low", "#111111", true}, {"normal", "#FFFFFF", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(t, script{})
			h.p.deps.Operator.Apply = func(ops []config.Op) ([]config.Change, error) {
				before, err := os.ReadFile(h.p.deps.Store.Path())
				if err != nil {
					return nil, err
				}
				after, changes, err := config.ApplyOps(before, ops, nil)
				if err != nil {
					return nil, err
				}
				if err := config.WriteFile(h.p.deps.Store.Path(), after); err != nil {
					return nil, err
				}
				h.p.deps.Store.Reload()
				return changes, nil
			}
			turn := &session.Turn{Reply: "已修改。\n```kami-operator\n" + `{"ops":[{"path":"themes","value":[{"id":"custom","base":"flexoki-night","colors":{"background":"#000000","text":"` + tc.text + `"}}]},{"path":"theme","value":"custom"}]}` + "\n```"}
			h.p.finishOperator(turn)
			if !turn.Undoable {
				t.Fatalf("ops not applied: %s", turn.Reply)
			}
			if got := strings.Contains(turn.Reply, "低于 4.5:1"); got != tc.warn {
				t.Fatalf("warning %v: %s", got, turn.Reply)
			}
			if h.p.cfg.Theme == "custom" {
				t.Fatal("test must read fresh store while panel config is stale")
			}
			// An edit that leaves the theme alone says nothing about it.
			other := &session.Turn{Reply: "已修改。\n```kami-operator\n" + `{"ops":[{"path":"launchAtLogin","value":true}]}` + "\n```"}
			h.p.finishOperator(other)
			if !other.Undoable || strings.Contains(other.Reply, "对比度") {
				t.Fatalf("non-theme edit: %s", other.Reply)
			}
		})
	}
}
