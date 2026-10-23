package capture

import (
	"cmp"
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Umupa/Kami/internal/macos"
)

// row is one visual line of the picture: the recognized lines that share a
// baseline, left to right, with the box around them.
type row struct {
	text                     string
	left, top, right, bottom float64
}

func (r row) height() float64 { return r.bottom - r.top }

// recognizedText turns what Vision read into the text a reader would have
// copied: a wrapped line rejoins its paragraph, while the breaks the author
// made (a blank line, a list item, a line that ends early) stay.
func recognizedText(lines []macos.Line) string {
	rows := readingRows(lines)
	if len(rows) == 0 {
		return ""
	}
	heights := make([]float64, len(rows))
	edge := 0.0
	for i, r := range rows {
		heights[i] = r.height()
		edge = max(edge, r.right)
	}
	slices.Sort(heights)
	lineHeight := heights[len(heights)/2]

	text := rows[0].text
	for i := 1; i < len(rows); i++ {
		if authorBreak(rows[i-1], rows[i], lineHeight, edge) {
			text += "\n" + rows[i].text
		} else {
			text = rejoin(text, rows[i].text)
		}
	}
	return strings.TrimSpace(text)
}

// readingRows groups lines whose vertical centers are within half a line of
// each other, top to bottom.
func readingRows(lines []macos.Line) []row {
	var kept []macos.Line
	for _, l := range lines {
		if l.Text = strings.TrimSpace(l.Text); l.Text != "" {
			kept = append(kept, l)
		}
	}
	center := func(l macos.Line) float64 { return l.Box.Y + l.Box.H/2 }
	slices.SortStableFunc(kept, func(a, b macos.Line) int { return cmp.Compare(center(a), center(b)) })

	var groups [][]macos.Line
	for _, l := range kept {
		if n := len(groups); n > 0 {
			prev := groups[n-1][len(groups[n-1])-1]
			if math.Abs(center(l)-center(prev)) < min(l.Box.H, prev.Box.H)/2 {
				groups[n-1] = append(groups[n-1], l)
				continue
			}
		}
		groups = append(groups, []macos.Line{l})
	}

	rows := make([]row, len(groups))
	for i, g := range groups {
		slices.SortStableFunc(g, func(a, b macos.Line) int { return cmp.Compare(a.Box.X, b.Box.X) })
		words := make([]string, len(g))
		r := row{left: math.Inf(1), top: math.Inf(1), right: math.Inf(-1), bottom: math.Inf(-1)}
		for j, l := range g {
			words[j] = l.Text
			r.left, r.top = min(r.left, l.Box.X), min(r.top, l.Box.Y)
			r.right, r.bottom = max(r.right, l.Box.X+l.Box.W), max(r.bottom, l.Box.Y+l.Box.H)
		}
		r.text = strings.Join(words, " ")
		rows[i] = r
	}
	return rows
}

// authorBreak reports whether next starts a new line in the original text
// rather than continuing prev: a gap larger than a line, a list item, or prev
// stopping short of the text's right edge although next's first word fits.
func authorBreak(prev, next row, lineHeight, edge float64) bool {
	if next.top-prev.bottom > lineHeight*0.8 || listItem(next.text) {
		return true
	}
	first, _ := utf8.DecodeRuneInString(next.text)
	word := 1
	if !isCJK(first) {
		word = strings.IndexFunc(next.text, unicode.IsSpace)
		if word < 0 {
			word = len(next.text)
		}
		word = utf8.RuneCountInString(next.text[:word])
	}
	glyph := (prev.right - prev.left) / float64(utf8.RuneCountInString(prev.text))
	return edge-prev.right > glyph*(float64(word)*1.4+1)
}

// listItem reports whether text begins with a bullet or a number of one to
// three digits followed by "、", ". " or ") ".
func listItem(text string) bool {
	first, n := utf8.DecodeRuneInString(text)
	switch {
	case strings.ContainsRune("•◦▪▫‣∙·●○■□▸►", first):
		return true
	case strings.ContainsRune("-*+", first):
		return strings.HasPrefix(text[n:], " ")
	}
	digits := strings.IndexFunc(text, func(r rune) bool { return r < '0' || r > '9' })
	if digits < 1 || digits > 3 {
		return false
	}
	for _, marker := range []string{"、", ". ", ") "} {
		if strings.HasPrefix(text[digits:], marker) {
			return true
		}
	}
	return false
}

// rejoin continues text with a wrapped line: directly next to Chinese or
// Japanese, without the hyphen of a word split across lines, and otherwise
// after a space.
func rejoin(text, next string) string {
	last, n := utf8.DecodeLastRuneInString(text)
	first, _ := utf8.DecodeRuneInString(next)
	if isCJK(last) || isCJK(first) {
		return text + next
	}
	if before, _ := utf8.DecodeLastRuneInString(text[:len(text)-n]); last == '-' && unicode.IsLetter(before) && unicode.IsLower(first) {
		return text[:len(text)-n] + next
	}
	return text + " " + next
}

// isCJK reports whether r belongs to a script written without spaces between
// words, or is its punctuation.
func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) ||
		r >= '　' && r <= '〿' || // CJK symbols and punctuation
		r >= '＀' && r <= '￯' // fullwidth forms
}
