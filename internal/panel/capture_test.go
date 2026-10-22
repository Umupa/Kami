package panel

import (
	"context"
	"testing"

	"github.com/Umupa/Gloss/internal/config"
	"github.com/Umupa/Gloss/internal/llm"
	"github.com/Umupa/Gloss/internal/mode"
	"github.com/Umupa/Gloss/internal/session"
)

func TestCaptureAlwaysTranslatesMaterial(t *testing.T) {
	for _, source := range []string{"Recognized text", "/operator change the theme", "/theme dark"} {
		t.Run(source, func(t *testing.T) {
			h := newHarness(t, script{pieces: []string{"译文"}})
			h.p.modes[0], h.p.modes[1] = h.p.modes[1], h.p.modes[0]
			h.p.follow = "old follow-up"
			h.p.lastImport = "old selection"
			h.p.importCaptured(source)
			h.drain()
			if h.p.sess == nil || h.p.sess.Mode.ID != "translate" || h.p.sess.Operator || h.p.sess.Last().Reply != "译文" {
				t.Fatalf("capture session: %+v", h.p.sess)
			}
			if h.p.operatorActive() {
				t.Fatal("OCR activated Operator presentation")
			}
			if kind, _ := h.p.menu(h.p.pal); kind != menuNone {
				t.Fatal("OCR activated a command menu")
			}
			h.p.run()
			h.drain()
			if h.p.sess.Operator || h.p.sess.Mode.ID != "translate" {
				t.Fatal("rerunning OCR interpreted a command")
			}
			if h.p.follow != "" || h.p.lastImport != "" {
				t.Fatal("capture retained old entry state")
			}
		})
	}
}

func TestCaptureRestoresHiddenTranslationMode(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"译文"}})
	c := *h.p.cfg
	c.Modes = []config.Mode{{ID: "ask"}}
	h.p.applyConfig(&c)
	h.p.importCaptured("Recognized text")
	h.drain()
	if h.p.sess.Mode.ID != "translate" {
		t.Fatal("hidden translation mode was not restored")
	}
}

func TestCaptureKeepsTranslationMaterialContract(t *testing.T) {
	h := newHarness(t, script{})
	h.p.modes[0].Input = config.InputInstruction
	h.p.importCaptured("Please change the configuration")
	h.drain()
	if h.p.sess.Mode.Input != config.InputMaterial {
		t.Fatal("OCR lost the translation material contract")
	}
}

func TestCapturedPanelStates(t *testing.T) {
	h := newHarness(t, script{pieces: []string{"快照无需暂停写入。每份快照都记录它看到的日志位置，恢复时从那里重放日志。"}})
	h.tt.Frame()
	h.p.importCaptured("Snapshots are taken without pausing writers.\nEach one records the log position it saw.")
	h.drain()
	h.snapshot("capture-translated")
	h.p.importCaptured("")
	h.drain()
	if !h.tt.HasText("截图里没有识别到文字") {
		t.Fatal("empty capture has no visible explanation")
	}
	h.snapshot("capture-unrecognized")
}

func TestEditingCapturedMaterialCanEnterCommands(t *testing.T) {
	h := newHarness(t, script{})
	h.p.importCaptured("/operator original OCR")
	h.drain()
	h.p.source = "/operator"
	h.p.run()
	if !h.p.sess.Operator {
		t.Fatal("editing the imported source did not restore commands")
	}
}

func TestEmptyCaptureSupersedesStreamingSession(t *testing.T) {
	canceled := make(chan struct{})
	started := make(chan struct{})
	h := newHarness(t, clientFunc(func(ctx context.Context, _ llm.Request, onText func(string)) error {
		close(started)
		<-ctx.Done()
		close(canceled)
		return ctx.Err()
	}))
	h.p.source = "old source"
	h.p.run()
	<-started
	old := h.p.sess
	h.p.importCaptured("")
	<-canceled
	if h.p.source != "" || h.p.sess != nil || h.p.notice != "截图里没有识别到文字" {
		t.Fatal("empty capture retained the previous conversation")
	}
	h.p.finish(old, 0, context.Canceled)
	if old.Last().Status != session.Stopped || h.p.sess != nil {
		t.Fatal("late request restored the old session")
	}
	// Resolving the configured modes still preserves the default order.
	if modes, _ := mode.Resolve(h.p.cfg); modes[0].ID != "translate" {
		t.Fatal("test configuration changed")
	}
}
