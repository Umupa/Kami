// Package segment splits long text at natural boundaries so that a request
// rejected as too long can be retried in pieces.
package segment

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Split cuts text into two parts near its middle, at the best boundary
// it can find, in this order of preference: a blank line between
// paragraphs; a line break; the end of a sentence (. ! ? followed by
// whitespace, or 。！？ ; also …); whitespace; anywhere between
// characters (never inside a UTF-8 sequence or a CRLF pair). Of the
// boundaries of the best kind, it takes the one closest to the middle,
// as long as it lies within the middle 80% of the text; otherwise it
// tries the next kind. The parts keep every byte: a + b == text.
// ok is false when text has fewer than 2 characters.
func Split(text string) (a, b string, ok bool) {
	if utf8.RuneCountInString(text) < 2 {
		return "", "", false
	}
	n := len(text)
	low, high := (n+9)/10, 9*n/10
	finders := []func(string) []int{
		blankLineBoundaries,
		lineBoundaries,
		sentenceBoundaries,
		whitespaceBoundaries,
		characterBoundaries,
	}
	for _, find := range finders {
		if pos, ok := closest(text, find(text), low, high); ok {
			return text[:pos], text[pos:], true
		}
	}
	return "", "", false
}

// closest returns the boundary position nearest the middle of text,
// among positions inside [low, high]. Ties take the earlier boundary.
func closest(text string, positions []int, low, high int) (int, bool) {
	n := len(text)
	best, bestDist := 0, 0
	found := false
	for _, pos := range positions {
		if pos <= 0 || pos >= n || pos < low || pos > high {
			continue
		}
		if text[pos-1] == '\r' && text[pos] == '\n' {
			continue // inside a CRLF pair
		}
		dist := 2*pos - n
		if dist < 0 {
			dist = -dist
		}
		if !found || dist < bestDist {
			best, bestDist, found = pos, dist, true
		}
	}
	return best, found
}

// blankLineBoundaries returns the positions just after lines that contain
// only whitespace.
func blankLineBoundaries(text string) []int {
	var out []int
	start := 0
	for i := 0; i < len(text); {
		switch text[i] {
		case '\n':
			if strings.TrimSpace(text[start:i]) == "" {
				out = append(out, i+1)
			}
			i++
		case '\r':
			end := i + 1
			if end < len(text) && text[end] == '\n' {
				end++
			}
			if strings.TrimSpace(text[start:i]) == "" {
				out = append(out, end)
			}
			i = end
		default:
			i++
			continue
		}
		start = i
	}
	return out
}

// lineBoundaries returns the positions just after each line break.
func lineBoundaries(text string) []int {
	var out []int
	for i := 0; i < len(text); i++ {
		switch {
		case text[i] == '\n':
			out = append(out, i+1)
		case text[i] == '\r' && (i+1 == len(text) || text[i+1] != '\n'):
			out = append(out, i+1)
		}
	}
	return out
}

// sentenceBoundaries returns the positions just after sentence-ending
// punctuation and the whitespace that follows it.
func sentenceBoundaries(text string) []int {
	var out []int
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if !sentenceEnd(r) {
			i += size
			continue
		}
		j, last := i+size, r
		for j < len(text) {
			r, size = utf8.DecodeRuneInString(text[j:])
			if !sentenceEnd(r) {
				break
			}
			j += size
			last = r
		}
		qualifies := last != '.' && last != '!' && last != '?'
		if !qualifies && j < len(text) {
			r, _ = utf8.DecodeRuneInString(text[j:])
			qualifies = unicode.IsSpace(r)
		}
		if qualifies {
			k := j
			for k < len(text) {
				r, size = utf8.DecodeRuneInString(text[k:])
				if !unicode.IsSpace(r) {
					break
				}
				k += size
			}
			out = append(out, k)
		}
		i = j
	}
	return out
}

// sentenceEnd reports whether r ends a sentence.
func sentenceEnd(r rune) bool {
	switch r {
	case '.', '!', '?', '。', '！', '？', '…':
		return true
	}
	return false
}

// whitespaceBoundaries returns the positions just after runs of whitespace.
func whitespaceBoundaries(text string) []int {
	var out []int
	for i := 0; i < len(text); {
		r, size := utf8.DecodeRuneInString(text[i:])
		if !unicode.IsSpace(r) {
			i += size
			continue
		}
		for i < len(text) {
			r, size = utf8.DecodeRuneInString(text[i:])
			if !unicode.IsSpace(r) {
				break
			}
			i += size
		}
		out = append(out, i)
	}
	return out
}

// characterBoundaries returns every byte position between UTF-8 runes.
func characterBoundaries(text string) []int {
	var out []int
	for i := range text {
		if i > 0 {
			out = append(out, i)
		}
	}
	return out
}
