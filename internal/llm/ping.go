package llm

import (
	"context"
	"strings"
	"time"

	"github.com/Umupa/Gloss/internal/config"
)

// Ping sends a model a one-word translation, the smallest request that shows
// the configuration works end to end. It returns the reply and how long it
// took; an empty reply is a failure.
func Ping(ctx context.Context, client Client, m config.Model, key string) (reply string, took time.Duration, err error) {
	req := PingRequest(m, key)
	var b strings.Builder
	start := time.Now()
	err = client.Stream(ctx, req, func(s string) { b.WriteString(s) })
	took = time.Since(start)
	reply = strings.TrimSpace(b.String())
	if err == nil && reply == "" {
		err = &Error{Kind: KindConfig, Detail: "the service answered with no text"}
	}
	return reply, took, err
}

// PingRequest is the request Ping sends, for showing it.
func PingRequest(m config.Model, key string) Request {
	return Request{
		Model:    m,
		Key:      key,
		System:   "Translate the user's message into Simplified Chinese. Reply with the translation only.",
		Messages: []Message{{Role: RoleUser, Content: "hello"}},
	}
}
