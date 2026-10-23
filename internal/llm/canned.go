package llm

import (
	"context"
	"time"
)

// Canned streams a reply the app wrote itself, a few words at a time, so
// that it comes in as a model's does.
type Canned struct {
	Text string
}

// cannedPiece and cannedPace are how much of a canned reply comes at a
// time, in bytes, and how often.
const (
	cannedPiece = 48
	cannedPace  = 16 * time.Millisecond
)

func (c Canned) Stream(ctx context.Context, _ Request, onText func(string)) error {
	reply := c.Text
	for len(reply) > 0 {
		n := min(len(reply), cannedPiece)
		for n < len(reply) && !utf8Start(reply[n]) {
			n++
		}
		onText(reply[:n])
		reply = reply[n:]
		if len(reply) == 0 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(cannedPace):
		}
	}
	return nil
}
