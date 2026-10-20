package session

import (
	"errors"
	"slices"
	"testing"

	"github.com/Umupa/Gloss/internal/llm"
	"github.com/Umupa/Gloss/internal/mode"
)

func TestNew(t *testing.T) {
	m := mode.Mode{ID: "translate", Name: "翻译"}
	s := New(m, "source text", "fast")
	if s.Mode != m {
		t.Errorf("Mode = %+v, want %+v", s.Mode, m)
	}
	if len(s.Turns) != 1 {
		t.Fatalf("len(Turns) = %d, want 1", len(s.Turns))
	}
	turn := s.Last()
	if turn.Ask != "source text" {
		t.Errorf("Ask = %q, want source text", turn.Ask)
	}
	if turn.Model != "fast" {
		t.Errorf("Model = %q, want fast", turn.Model)
	}
	if turn.Reply != "" {
		t.Errorf("Reply = %q, want empty", turn.Reply)
	}
	if turn.Status != Streaming {
		t.Errorf("Status = %v, want Streaming", turn.Status)
	}
	if turn.Err != nil {
		t.Errorf("Err = %v, want nil", turn.Err)
	}
	if !s.Busy() {
		t.Error("Busy() = false for a new session")
	}
	want := []llm.Message{{Role: llm.RoleUser, Content: "source text"}}
	if got := s.Messages(); !slices.Equal(got, want) {
		t.Errorf("Messages() = %v, want %v", got, want)
	}
}

func TestFollowUpAndBusy(t *testing.T) {
	s := New(mode.Mode{ID: "ask"}, "first", "fast")
	if !s.Busy() {
		t.Error("Busy() = false for a streaming first turn")
	}
	s.Last().Status = Done
	if s.Busy() {
		t.Error("Busy() = true after the first turn ended")
	}

	s.FollowUp("second", "strong")
	if len(s.Turns) != 2 {
		t.Fatalf("len(Turns) = %d, want 2", len(s.Turns))
	}
	turn := s.Last()
	if turn.Ask != "second" || turn.Model != "strong" {
		t.Errorf("latest turn = %+v, want ask second on strong", turn)
	}
	if turn.Reply != "" || turn.Status != Streaming || turn.Err != nil {
		t.Errorf("latest turn was not started clean: %+v", turn)
	}
	if !s.Busy() {
		t.Error("Busy() = false for a new follow-up")
	}
}

func TestMessagesThreeTurns(t *testing.T) {
	s := New(mode.Mode{ID: "ask"}, "first", "m1")
	s.Last().Reply = "first reply"
	s.Last().Status = Done

	s.FollowUp("second", "m2")
	s.Last().Reply = "second reply"
	s.Last().Status = Done

	s.FollowUp("third", "m3")
	// The latest reply is not part of its own request, even if one is set.
	s.Last().Reply = "draft reply"

	wantTurns := []struct {
		ask, model, reply string
		status            Status
	}{
		{"first", "m1", "first reply", Done},
		{"second", "m2", "second reply", Done},
		{"third", "m3", "draft reply", Streaming},
	}
	if len(s.Turns) != len(wantTurns) {
		t.Fatalf("len(Turns) = %d, want %d", len(s.Turns), len(wantTurns))
	}
	for i, want := range wantTurns {
		got := s.Turns[i]
		if got.Ask != want.ask || got.Model != want.model || got.Reply != want.reply || got.Status != want.status {
			t.Errorf("Turns[%d] = %+v, want %+v", i, got, want)
		}
	}

	want := []llm.Message{
		{Role: llm.RoleUser, Content: "first"},
		{Role: llm.RoleAssistant, Content: "first reply"},
		{Role: llm.RoleUser, Content: "second"},
		{Role: llm.RoleAssistant, Content: "second reply"},
		{Role: llm.RoleUser, Content: "third"},
	}
	if got := s.Messages(); !slices.Equal(got, want) {
		t.Errorf("Messages() = %v, want %v", got, want)
	}
}

func TestRetry(t *testing.T) {
	s := New(mode.Mode{ID: "translate"}, "source", "m1")
	s.Last().Reply = "first reply"
	s.Last().Status = Done

	s.FollowUp("follow up", "m2")
	s.Last().Reply = "wrong reply"
	s.Last().Status = Failed
	s.Last().Err = errors.New("boom")

	s.Retry()
	turn := s.Last()
	if turn.Ask != "follow up" || turn.Model != "m2" {
		t.Errorf("Retry changed the request: %+v", turn)
	}
	if turn.Reply != "" {
		t.Errorf("Reply = %q after Retry, want empty", turn.Reply)
	}
	if turn.Err != nil {
		t.Errorf("Err = %v after Retry, want nil", turn.Err)
	}
	if turn.Status != Streaming {
		t.Errorf("Status = %v after Retry, want Streaming", turn.Status)
	}
	if !s.Busy() {
		t.Error("Busy() = false after Retry")
	}

	first := s.Turns[0]
	if first.Ask != "source" || first.Model != "m1" || first.Reply != "first reply" || first.Status != Done || first.Err != nil {
		t.Errorf("Retry changed the earlier turn: %+v", first)
	}
	want := []llm.Message{
		{Role: llm.RoleUser, Content: "source"},
		{Role: llm.RoleAssistant, Content: "first reply"},
		{Role: llm.RoleUser, Content: "follow up"},
	}
	if got := s.Messages(); !slices.Equal(got, want) {
		t.Errorf("Messages() = %v, want %v", got, want)
	}
}
