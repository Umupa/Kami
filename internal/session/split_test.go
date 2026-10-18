package session

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Umupa/cida-next/internal/llm"
	"github.com/Umupa/cida-next/internal/segment"
)

var errTooLong = &llm.Error{Kind: llm.KindTooLong, Detail: "too long"}

// echo answers a part with its upper-cased text, refusing parts longer than
// limit bytes.
func echo(limit int, sent *[]string) SendPart {
	return func(ctx context.Context, part string, onText func(string)) error {
		if len(part) > limit {
			return errTooLong
		}
		*sent = append(*sent, part)
		onText(strings.ToUpper(strings.TrimSpace(part)))
		return nil
	}
}

func TestStreamPartsCutsUntilEachFits(t *testing.T) {
	src := "one two three.\n\nfour five six.\n\nseven eight nine.\n\nten eleven twelve."
	var sent []string
	var out strings.Builder
	n, err := StreamParts(context.Background(), src, segment.Split, echo(20, &sent), func(s string) { out.WriteString(s) })
	if err != nil {
		t.Fatal(err)
	}
	if n != len(sent) || n < 4 {
		t.Errorf("parts = %d, sent %q", n, sent)
	}
	if strings.Join(sent, "") != src {
		t.Errorf("the parts do not add up to the source: %q", sent)
	}
	want := "ONE TWO THREE.\n\nFOUR FIVE SIX.\n\nSEVEN EIGHT NINE.\n\nTEN ELEVEN TWELVE."
	if out.String() != want {
		t.Errorf("reply = %q, want %q", out.String(), want)
	}
}

func TestStreamPartsGivesUpOnAnUncuttablePart(t *testing.T) {
	var sent []string
	_, err := StreamParts(context.Background(), "ab", segment.Split, echo(0, &sent), func(string) {})
	if !TooLong(err) {
		t.Errorf("err = %v, want too long", err)
	}
}

func TestStreamPartsStopsOnOtherErrors(t *testing.T) {
	boom := errors.New("boom")
	calls := 0
	send := func(ctx context.Context, part string, onText func(string)) error {
		calls++
		if calls == 2 {
			return boom
		}
		onText("ok")
		return nil
	}
	var out strings.Builder
	n, err := StreamParts(context.Background(), "first part.\n\nsecond part.", segment.Split, send, func(s string) { out.WriteString(s) })
	if !errors.Is(err, boom) || n != 1 || out.String() != "ok" {
		t.Errorf("n=%d err=%v out=%q", n, err, out.String())
	}
}
