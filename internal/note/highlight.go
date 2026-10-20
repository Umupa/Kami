package note

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Kind is what a span of a note's Markdown is.
type Kind int

const (
	Heading Kind = iota + 1
	// Mark is a list's bullet, a task's box, a quote's >, a heading's #.
	Mark
	Strong
	Code
	Quote
	Link
)

// Span is a run of a note's text, in runes, [Start, End).
type Span struct {
	Start, End int
	Kind       Kind
}

var (
	headingRe = regexp.MustCompile(`^(#{1,6})\s`)
	listRe    = regexp.MustCompile(`^(\s*)([-*+]|\d+[.)])(\s+\[[ xX]\])?\s`)
	strongRe  = regexp.MustCompile(`\*\*[^*\n]+\*\*|__[^_\n]+__`)
	codeRe    = regexp.MustCompile("`[^`\n]+`")
	// A URL ends at the first space, bracket, quote or non-ASCII rune, as
	// where Chinese text goes on after it.
	urlRe = regexp.MustCompile(`https?://[^\s<>()\[\]{}"'` + "`" + `\x{80}-\x{10FFFF}]+`)
)

// Highlight finds the Markdown of a note to color: it never changes the
// text, only says what its runs are. Spans may nest, the inner after the
// outer.
func Highlight(text string) []Span {
	var spans []Span
	at := 0 // the rune where the line starts
	fence := false
	for line := range strings.Lines(text) {
		body := strings.TrimRight(line, "\n")
		add := func(start, end int, k Kind) {
			// start and end are byte offsets into body.
			s := at + utf8.RuneCountInString(body[:start])
			spans = append(spans, Span{s, s + utf8.RuneCountInString(body[start:end]), k})
		}
		trimmed := strings.TrimSpace(body)
		switch {
		case strings.HasPrefix(trimmed, "```"):
			fence = !fence
			add(0, len(body), Code)
		case fence:
			if body != "" {
				add(0, len(body), Code)
			}
		default:
			if m := headingRe.FindStringSubmatchIndex(body); m != nil {
				add(0, len(body), Heading)
				add(m[2], m[3], Mark)
			} else if strings.HasPrefix(trimmed, ">") {
				add(0, len(body), Quote)
				i := strings.Index(body, ">")
				add(i, i+1, Mark)
			} else if m := listRe.FindStringSubmatchIndex(body); m != nil {
				end := m[5]
				if m[6] >= 0 {
					end = m[7]
				}
				add(m[4], end, Mark)
			}
			for _, m := range strongRe.FindAllStringIndex(body, -1) {
				add(m[0], m[1], Strong)
			}
			for _, m := range codeRe.FindAllStringIndex(body, -1) {
				add(m[0], m[1], Code)
			}
			for _, m := range urlRe.FindAllStringIndex(body, -1) {
				add(m[0], m[0]+len(trimURL(body[m[0]:m[1]])), Link)
			}
		}
		at += utf8.RuneCountInString(line)
	}
	return spans
}

// URLAt returns the URL around the rune at offset, "" when there is none.
func URLAt(text string, offset int) string {
	at := 0
	for line := range strings.Lines(text) {
		n := utf8.RuneCountInString(line)
		if offset >= at && offset <= at+n {
			for _, m := range urlRe.FindAllStringIndex(line, -1) {
				url := trimURL(line[m[0]:m[1]])
				start := at + utf8.RuneCountInString(line[:m[0]])
				if offset >= start && offset <= start+utf8.RuneCountInString(url) {
					return url
				}
			}
			return ""
		}
		at += n
	}
	return ""
}

// trimURL leaves out the punctuation that ends a sentence after a URL.
func trimURL(url string) string {
	return strings.TrimRight(url, ".,;:!?。，；：！？、")
}
