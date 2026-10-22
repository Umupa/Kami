// Package session holds the conversation of the panel: the source, the mode
// it runs in, and its turns. It lives in memory only and is never written to
// disk.
package session

import (
	"github.com/Umupa/Kami/internal/llm"
	"github.com/Umupa/Kami/internal/mode"
)

// Status is the state of a turn.
type Status int

const (
	Streaming Status = iota
	Done
	Stopped
	Failed
)

// Turn is one request and its reply. The first turn's request is the
// session's source; later ones are follow-ups.
type Turn struct {
	Ask    string
	Reply  string
	Model  string
	Status Status
	Err    error
	// Dropped is how many earlier turns were left out of the request to fit
	// the model's context; Parts how many parts a too-long source was
	// answered in, 0 when it went whole.
	Dropped, Parts int
	// Local marks a reply the app wrote itself, without a model.
	Local bool
	// Undoable marks an operator reply whose settings change can be undone.
	Undoable bool
}

// Session is a conversation. Its methods are not safe for concurrent use:
// the panel changes it on the main thread only.
type Session struct {
	Mode  mode.Mode
	Turns []Turn
	// Operator marks a session about the app's own settings; its
	// follow-ups stay in operator mode.
	Operator bool
}

// New starts a session in m with source as its first request.
func New(m mode.Mode, source, model string) *Session {
	return &Session{Mode: m, Turns: []Turn{{Ask: source, Model: model}}}
}

// Last returns the latest turn.
func (s *Session) Last() *Turn { return &s.Turns[len(s.Turns)-1] }

// Busy reports whether the latest turn is still streaming.
func (s *Session) Busy() bool { return s.Last().Status == Streaming }

// FollowUp adds a turn asking ask, after the latest turn has ended.
func (s *Session) FollowUp(ask, model string) {
	s.Turns = append(s.Turns, Turn{Ask: ask, Model: model})
}

// Retry clears the latest turn's reply so that it runs again.
func (s *Session) Retry() {
	t := s.Last()
	t.Reply, t.Status, t.Err, t.Dropped, t.Parts = "", Streaming, nil, 0, 0
}

// Messages returns the conversation up to the latest turn, for its request:
// every earlier turn's request and reply, then the latest request.
func (s *Session) Messages() []llm.Message {
	msgs := make([]llm.Message, 0, 2*len(s.Turns))
	for i, t := range s.Turns {
		msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: t.Ask})
		if i < len(s.Turns)-1 {
			msgs = append(msgs, llm.Message{Role: llm.RoleAssistant, Content: t.Reply})
		}
	}
	return msgs
}
