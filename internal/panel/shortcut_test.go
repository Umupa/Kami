package panel

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/session"
)

func TestModeShortcutRunsWithSource(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"ok"}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("hello")
	h.tt.Key(ui.Cmd, ui.Key2)
	h.drain()

	if h.p.modeIdx != 1 {
		t.Fatalf("mode %d after Cmd-2, want improve", h.p.modeIdx)
	}
	if h.p.sess == nil || h.p.sess.Mode.ID != h.p.modes[1].ID {
		t.Fatalf("session %+v after Cmd-2, want a run in improve", h.p.sess)
	}
}

func TestModeShortcutOnlySwitchesWithEmptySource(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Key(ui.Cmd, ui.Key3)
	h.tt.Frame()

	if h.p.modeIdx != 2 {
		t.Fatalf("mode %d after Cmd-3, want ask", h.p.modeIdx)
	}
	if h.p.sess != nil {
		t.Fatalf("Cmd-3 with an empty source started %+v", h.p.sess)
	}
}

func TestModeShortcutOutOfRangeDoesNothing(t *testing.T) {
	h := newHarness(t, script{})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("hello")
	h.tt.Key(ui.Cmd, ui.Key9)
	h.tt.Frame()

	if h.p.modeIdx != 0 || h.p.sess != nil {
		t.Fatalf("Cmd-9 left mode %d with session %+v, want no change", h.p.modeIdx, h.p.sess)
	}
}

func TestModeShortcutIgnoresNotes(t *testing.T) {
	h := newHarness(t, script{})
	h.p.setMode(len(h.p.modes))
	h.tt.Frame()
	h.tt.Key(ui.Cmd, ui.Key1)
	h.tt.Frame()

	if !h.p.noteMode() {
		t.Fatalf("Cmd-1 left notes for mode %d", h.p.modeIdx)
	}
}

func TestModeShortcutIgnoresOperator(t *testing.T) {
	h := newHarness(t, script{})
	h.p.setMode(1)
	h.p.source = "/operator theme"
	h.tt.Frame()
	h.tt.Key(ui.Cmd, ui.Key3)
	h.tt.Frame()

	if h.p.modeIdx != 1 || h.p.sess != nil {
		t.Fatalf("Cmd-3 in operator mode left mode %d with session %+v", h.p.modeIdx, h.p.sess)
	}
}

func TestModeShortcutWithoutModelOnlySwitches(t *testing.T) {
	h := newHarnessWith(t, script{}, `{"version":1}`)
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("hello")
	h.tt.Key(ui.Cmd, ui.Key2)
	h.tt.Frame()

	if h.p.modeIdx != 1 {
		t.Fatalf("mode %d after Cmd-2, want improve", h.p.modeIdx)
	}
	if h.p.sess != nil {
		t.Fatalf("Cmd-2 without a model started %+v", h.p.sess)
	}
}

func TestModeShortcutOnUpdateScreenOnlySwitches(t *testing.T) {
	h := newUpdateHarness(t)
	h.p.openUpdate()
	h.p.source = "hello"
	h.tt.Frame()
	h.tt.Key(ui.Cmd, ui.Key2)
	h.tt.Frame()

	if h.p.modeIdx != 1 || h.p.sess != nil {
		t.Fatalf("Cmd-2 on the update screen left mode %d with session %+v", h.p.modeIdx, h.p.sess)
	}
}

func TestCopyAndHide(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"**reply**"}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("question")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()

	hidden := 0
	h.p.hideWindow = func() { hidden++ }
	h.clip = ""
	h.tt.Key(ui.Cmd, ui.KeyEnter)

	if h.clip != "**reply**" {
		t.Fatalf("Cmd-Enter copied %q, want the reply's Markdown", h.clip)
	}
	if hidden != 1 {
		t.Fatalf("Cmd-Enter hid the panel %d times, want once", hidden)
	}
}

func TestCopyAndHideFromFollowUp(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"reply"}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("question")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	h.tt.Key(ui.Cmd, ui.KeyJ)
	if !h.tt.Focused("追问") {
		t.Fatal("Cmd-J did not focus the follow-up")
	}

	hidden := 0
	h.p.hideWindow = func() { hidden++ }
	h.clip = ""
	h.tt.Key(ui.Cmd, ui.KeyEnter)

	if h.clip != "reply" || hidden != 1 {
		t.Fatalf("Cmd-Enter from the follow-up copied %q and hid %d times", h.clip, hidden)
	}
}

func TestCopyAndHideKeepsPinnedPanel(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"reply"}})
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("question")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()

	hidden := 0
	h.p.hideWindow = func() { hidden++ }
	h.p.pinned = true
	h.clip = ""
	h.tt.Key(ui.Cmd, ui.KeyEnter)

	if h.clip != "reply" {
		t.Fatalf("Cmd-Enter on a pinned panel copied %q, want the reply", h.clip)
	}
	if hidden != 0 {
		t.Fatalf("Cmd-Enter hid a pinned panel %d times", hidden)
	}
}

func TestCopyAndHideWithoutResultDoesNothing(t *testing.T) {
	h := newHarness(t, script{})
	hidden := 0
	h.p.hideWindow = func() { hidden++ }
	h.tt.Frame()
	h.clip = ""
	h.tt.Key(ui.Cmd, ui.KeyEnter)

	if h.clip != "" || hidden != 0 {
		t.Fatalf("Cmd-Enter without a result copied %q and hid %d times", h.clip, hidden)
	}
}

func TestCopyAndHideWhileStreamingDoesNothing(t *testing.T) {
	h := newHarness(t, script{})
	h.p.sess = &session.Session{Turns: []session.Turn{{Reply: "partial", Status: session.Streaming}}}
	h.tt.Frame()
	hidden := 0
	h.p.hideWindow = func() { hidden++ }
	h.clip = ""
	h.tt.Key(ui.Cmd, ui.KeyEnter)

	if h.clip != "" || hidden != 0 {
		t.Fatalf("Cmd-Enter while streaming copied %q and hid %d times", h.clip, hidden)
	}
}

func TestCopyAndHideIgnoresNotes(t *testing.T) {
	h := newHarness(t, script{})
	h.p.sess = &session.Session{Turns: []session.Turn{{Reply: "reply", Status: session.Done}}}
	h.p.setMode(len(h.p.modes))
	h.tt.Frame()
	hidden := 0
	h.p.hideWindow = func() { hidden++ }
	h.clip = ""
	h.tt.Key(ui.Cmd, ui.KeyEnter)

	if h.clip != "" || hidden != 0 {
		t.Fatalf("Cmd-Enter in notes copied %q and hid %d times", h.clip, hidden)
	}
}
