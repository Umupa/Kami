package session

import (
	"context"
	"errors"

	"github.com/Umupa/Gloss/internal/llm"
)

// SendPart streams the reply to one part of a source.
type SendPart func(ctx context.Context, part string, onText func(string)) error

// StreamParts answers a source too long for one request (F7.3): it cuts the
// source in two with split, answers each part on its own, and streams the
// replies in order, a blank line apart. A part still too long is cut again;
// parts already answered are never sent again. It returns how many parts
// were sent successfully.
func StreamParts(ctx context.Context, source string, split func(string) (string, string, bool), send SendPart, onText func(string)) (int, error) {
	a, b, ok := split(source)
	if !ok {
		return 0, &llm.Error{Kind: llm.KindTooLong, Detail: "the source cannot be cut further"}
	}
	queue := []string{a, b}
	done := 0
	for len(queue) > 0 {
		part := queue[0]
		queue = queue[1:]
		// Services refuse a request as too long before replying, so a part
		// is cut again only when nothing of its reply came.
		replied := false
		err := send(ctx, part, func(s string) {
			if !replied && done > 0 {
				onText("\n\n")
			}
			replied = true
			onText(s)
		})
		if TooLong(err) && !replied {
			a, b, ok := split(part)
			if !ok {
				return done, err
			}
			queue = append([]string{a, b}, queue...)
			continue
		}
		if err != nil {
			return done, err
		}
		done++
	}
	return done, nil
}

// TooLong reports a request the service refused as too long.
func TooLong(err error) bool {
	var le *llm.Error
	return errors.As(err, &le) && le.Kind == llm.KindTooLong
}
