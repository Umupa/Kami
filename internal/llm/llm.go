// Package llm streams replies from model services in the three wire formats
// of config.Format, and sorts their failures into what the user can do about
// them.
package llm

import (
	"context"

	"github.com/Umupa/Gloss/internal/config"
)

// Role is who wrote a message.
type Role string

const (
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is one message of a conversation.
type Message struct {
	Role    Role
	Content string
}

// Request is one streamed completion.
type Request struct {
	Model config.Model
	// Key is the API key, empty when the service takes none.
	Key      string
	System   string
	Messages []Message
}

// Client streams completions.
type Client interface {
	// Stream sends req and calls onText, on the calling goroutine, with each
	// piece of the reply as it arrives. It returns nil once the reply is
	// complete, ctx.Err() when ctx ends first, and an *Error otherwise.
	Stream(ctx context.Context, req Request, onText func(string)) error
}
