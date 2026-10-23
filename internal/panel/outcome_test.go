package panel

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/llm"
)

func TestOutcomeReportsRequestsOnly(t *testing.T) {
	var outcomes []error
	rejected := &llm.Error{Kind: llm.KindConfig, Status: 401}
	fail := false
	h := newHarnessWith(t, clientFunc(func(_ context.Context, _ llm.Request, emit func(string)) error {
		if fail {
			return rejected
		}
		emit("Bonjour")
		return nil
	}), testConfig)
	h.p.deps.OnOutcome = func(err error) { outcomes = append(outcomes, err) }
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Type("Hello")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	fail = true
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	if len(outcomes) != 2 || outcomes[0] != nil || outcomes[1] != rejected {
		t.Fatalf("outcomes %v: want an answer, then the rejection", outcomes)
	}

	// Kami's own reply is not a request.
	setup := newSetupHarness(t, script{}, `{"version":1}`)
	setup.p.deps.OnOutcome = func(err error) { t.Errorf("the first-run reply reported %v", err) }
	setup.show()
}
