package session

import (
	"unicode"
	"unicode/utf8"

	"github.com/Umupa/Kami/internal/llm"
)

// messageOverhead is the per-message cost of roles and framing that chat
// formats add around the content.
const messageOverhead = 4

// EstimateTokens estimates the tokens of s for any model's tokenizer,
// erring high: each CJK character (Han, Hiragana, Katakana, Hangul,
// fullwidth forms) counts 1; other text counts 1 per 3 bytes, rounded up
// per run; at least 1 for a non-empty string.
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	tokens, other := 0, 0
	for len(s) > 0 {
		r, size := utf8.DecodeRuneInString(s)
		if isCJK(r) {
			tokens += (other + 2) / 3
			other = 0
			tokens++
		} else {
			other += size
		}
		s = s[size:]
	}
	return tokens + (other+2)/3
}

// isCJK reports whether r is a CJK character or part of a CJK block that a
// tokenizer is likely to count per character.
func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Bopomofo) ||
		r >= '\u3000' && r <= '\u303f' || // CJK symbols and punctuation
		r >= '\uff00' && r <= '\uffef' // halfwidth and fullwidth forms
}

// Trim fits a conversation into budget tokens: it keeps the first user
// message (the source), its reply when there is one, and as many of the
// latest messages as fit, dropping whole turns from the middle. The last
// message is always kept. It returns the messages to send and how many
// turns it dropped. Each message also costs 4 tokens of overhead.
// With budget <= 0, or when everything fits, it returns msgs unchanged.
func Trim(msgs []llm.Message, budget int) (kept []llm.Message, droppedTurns int) {
	if budget <= 0 || len(msgs) <= 1 || !trimmable(msgs) {
		return msgs, 0
	}
	total := 0
	for _, m := range msgs {
		total += messageTokens(m)
	}
	if total <= budget {
		return msgs, 0
	}

	n := len(msgs)
	middle := (n - 3) / 2
	cost := messageTokens(msgs[0]) + messageTokens(msgs[1]) + messageTokens(msgs[n-1])
	if cost > budget {
		// The source turn stays even without its reply; only the middle
		// turns count as dropped.
		return []llm.Message{msgs[0], msgs[n-1]}, middle
	}

	keep := 0
	for keep < middle {
		i := n - 3 - 2*keep
		pair := messageTokens(msgs[i]) + messageTokens(msgs[i+1])
		if cost+pair > budget {
			break
		}
		cost += pair
		keep++
	}
	kept = make([]llm.Message, 0, 2+2*keep)
	kept = append(kept, msgs[0], msgs[1])
	kept = append(kept, msgs[n-1-2*keep:]...)
	return kept, middle - keep
}

// trimmable reports whether msgs has the shape Trim can shorten: an
// alternating conversation that starts with the user and ends with the
// request. Anything else is returned unchanged.
func trimmable(msgs []llm.Message) bool {
	if len(msgs) < 3 || len(msgs)%2 == 0 {
		return false
	}
	for i, m := range msgs {
		want := llm.RoleUser
		if i%2 == 1 {
			want = llm.RoleAssistant
		}
		if m.Role != want {
			return false
		}
	}
	return true
}

func messageTokens(m llm.Message) int {
	return messageOverhead + EstimateTokens(m.Content)
}
