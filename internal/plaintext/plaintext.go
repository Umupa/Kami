// Package plaintext renders Markdown as text for places that show none.
package plaintext

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/egoist/mygo/ui/markdown/parser"
)

// FromMarkdown returns the text of a Markdown document without its syntax,
// laid out to read well where Markdown is not rendered: chat apps, mail,
// plain text fields.
func FromMarkdown(src string) string {
	d := parser.Parse(src)
	r := &renderer{d: d, numbers: map[int]int{}}
	r.numberFootnotes()
	lines := r.blocks(0, 0)
	if notes := r.footnotes(); len(notes) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, notes...)
	}
	return finish(lines)
}

// renderer turns the blocks and inlines of a document into lines. It
// carries the numbering of footnotes while it walks.
type renderer struct {
	d       *parser.Document
	numbers map[int]int // footnote definition block → its number
	defs    []int       // footnote definition blocks, in document order
}

// numberFootnotes numbers the definitions by their order in the document,
// as the Markdown view does.
func (r *renderer) numberFootnotes() {
	for i := 1; i < r.d.Len(); i++ {
		if r.d.Block(i).Kind != parser.FootnoteDefinition {
			continue
		}
		r.numbers[i] = len(r.defs) + 1
		r.defs = append(r.defs, i)
	}
}

// blocks renders the children of block parent, each block separated from
// the next by one blank line. It leaves footnote definitions out: they
// come at the end (see footnotes).
func (r *renderer) blocks(parent, depth int) []string {
	var out []string
	for i := range r.d.Children(parent) {
		if r.d.Block(i).Kind == parser.FootnoteDefinition {
			continue
		}
		lines := r.block(i, depth)
		if len(lines) == 0 {
			continue
		}
		if len(out) > 0 {
			out = append(out, "")
		}
		out = append(out, lines...)
	}
	return out
}

// block renders block i. depth is the number of lists around it.
func (r *renderer) block(i, depth int) []string {
	b := r.d.Block(i)
	switch b.Kind {
	case parser.Paragraph, parser.Heading:
		return textLines(r.inlines(i))
	case parser.ThematicBreak:
		return nil
	case parser.CodeBlock, parser.HTMLBlock, parser.MathBlock:
		return rawLines(r.d.Lines(i))
	case parser.Quote:
		return r.blocks(i, depth)
	case parser.List:
		return r.list(i, depth)
	case parser.Table:
		return r.table(i)
	case parser.Item:
		return r.item(i, depth, "•")
	case parser.FootnoteDefinition:
		return nil
	}
	return nil
}

// list renders a list whose items stand at depth.
func (r *renderer) list(i, depth int) []string {
	b := r.d.Block(i)
	number := b.Number
	var out []string
	for k, item := range items(r.d, i) {
		if k > 0 && !b.Tight {
			out = append(out, "")
		}
		it := r.d.Block(item)
		marker := "•"
		switch {
		case it.Task == parser.Unchecked:
			marker = "☐"
		case it.Task == parser.Checked:
			marker = "☑"
		case b.Ordered:
			marker = fmt.Sprintf("%d.", number)
		}
		out = append(out, r.item(item, depth, marker)...)
		number++
	}
	return out
}

// item renders a list item. Its first paragraph follows the marker; its
// other content is indented to where the text after the marker starts.
// A nested list is indented two spaces per level.
func (r *renderer) item(i, depth int, marker string) []string {
	b := r.d.Block(i)
	base := strings.Repeat("  ", depth)
	content := base + strings.Repeat(" ", utf8.RuneCountInString(marker)+1)
	children := itemChildren(r.d, i)
	if len(children) == 0 {
		return []string{base + marker}
	}
	tight := true
	if b.Parent > 0 {
		if list := r.d.Block(b.Parent); list.Kind == parser.List {
			tight = list.Tight
		}
	}
	var out []string
	for k, child := range children {
		cb := r.d.Block(child)
		nested := cb.Kind == parser.List
		childDepth := depth
		if nested {
			childDepth++
		}
		lines := r.block(child, childDepth)
		if k == 0 {
			if (cb.Kind == parser.Paragraph || cb.Kind == parser.Heading) && len(lines) > 0 {
				out = append(out, base+marker+" "+lines[0])
				for _, line := range lines[1:] {
					out = append(out, indentLine(content, line))
				}
			} else {
				out = append(out, base+marker)
				out = append(out, indentLines(content, lines, nested)...)
			}
			continue
		}
		if !tight {
			out = append(out, "")
		}
		out = append(out, indentLines(content, lines, nested)...)
	}
	return out
}

// table renders a table as one tab-separated line per row, the header
// first and the delimiter row left out.
func (r *renderer) table(i int) []string {
	var out []string
	for row := range r.d.Children(i) {
		var cells []string
		for cell := range r.d.Children(row) {
			cells = append(cells, strings.ReplaceAll(r.inlines(cell), "\n", " "))
		}
		out = append(out, strings.Join(cells, "\t"))
	}
	return out
}

// footnotes renders the definitions at the end, numbered as references
// point to them.
func (r *renderer) footnotes() []string {
	var out []string
	for _, block := range r.defs {
		if len(out) > 0 {
			out = append(out, "")
		}
		n := r.numbers[block]
		prefix := fmt.Sprintf("[%d] ", n)
		body := r.blocks(block, 0)
		if len(body) == 0 {
			out = append(out, strings.TrimSuffix(prefix, " "))
			continue
		}
		out = append(out, prefix+body[0])
		pad := strings.Repeat(" ", utf8.RuneCountInString(prefix))
		for _, line := range body[1:] {
			out = append(out, indentLine(pad, line))
		}
	}
	return out
}

// inlines returns the text of block i's inlines, parsed the first time.
func (r *renderer) inlines(i int) string {
	in := r.d.Inlines(i)
	return renderAtoms(r.atoms(in, 0, len(in)))
}

// atom is a piece of inline text, with breaks kept apart so that the
// space they become can depend on the text around them.
type atom struct {
	kind atomKind
	text string
}

type atomKind uint8

const (
	textAtom atomKind = iota
	softBreakAtom
	hardBreakAtom
)

// atoms flattens the inlines [from, to) into text, soft breaks and hard
// breaks, leaving out the syntax of emphasis, code, links and HTML.
func (r *renderer) atoms(in []parser.Inline, from, to int) []atom {
	var out []atom
	for k := from; k < to; k = in[k].Next {
		n := &in[k]
		switch n.Kind {
		case parser.Text, parser.Code, parser.Math:
			out = append(out, atom{text: n.Text})
		case parser.SoftBreak:
			out = append(out, atom{kind: softBreakAtom})
		case parser.LineBreak:
			out = append(out, atom{kind: hardBreakAtom})
		case parser.Emphasis, parser.Strong, parser.Strikethrough:
			out = append(out, r.atoms(in, k+1, n.Next)...)
		case parser.Link:
			label := r.atoms(in, k+1, n.Next)
			text := renderAtoms(label)
			switch {
			case n.Dest == "", n.Auto, text == n.Dest:
				out = append(out, label...)
			case text == "":
				out = append(out, atom{text: n.Dest})
			default:
				out = append(out, label...)
				out = append(out, atom{text: " (" + n.Dest + ")"})
			}
		case parser.Image:
			out = append(out, r.atoms(in, k+1, n.Next)...)
		case parser.HTML:
			// Inline tags carry no text of their own.
		case parser.FootnoteRef:
			number := 0
			if block, ok := r.d.Footnote(n.Text); ok {
				number = r.numbers[block]
			}
			out = append(out, atom{text: fmt.Sprintf("[%d]", number)})
		}
	}
	return out
}

// renderAtoms joins atoms, turning soft breaks into a space in Latin text
// and into nothing between two CJK characters, and hard breaks into
// newlines.
func renderAtoms(atoms []atom) string {
	var b strings.Builder
	var last rune
	haveLast := false
	write := func(s string) {
		if s == "" {
			return
		}
		b.WriteString(s)
		last, _ = utf8.DecodeLastRuneInString(s)
		haveLast = true
	}
	for k, a := range atoms {
		switch a.kind {
		case textAtom:
			write(a.text)
		case hardBreakAtom:
			write("\n")
		case softBreakAtom:
			if !haveLast {
				continue
			}
			next, ok := nextRune(atoms, k+1)
			switch {
			case !ok, unicode.IsSpace(last), unicode.IsSpace(next):
			case isCJK(last) && isCJK(next):
			default:
				write(" ")
			}
		}
	}
	return b.String()
}

// nextRune returns the first rune of the text after atoms[from], or false
// when a break or nothing comes first.
func nextRune(atoms []atom, from int) (rune, bool) {
	for _, a := range atoms[from:] {
		if a.kind != textAtom {
			return 0, false
		}
		if a.text != "" {
			r, _ := utf8.DecodeRuneInString(a.text)
			return r, true
		}
	}
	return 0, false
}

// isCJK reports whether r is written in a script that does not separate
// words with spaces.
func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Bopomofo) ||
		r >= '\u3000' && r <= '\u303f' || // CJK symbols and punctuation
		r >= '\uff00' && r <= '\uffef' // fullwidth forms
}

// itemChildren returns the children of item i, leaving out footnote
// definitions, which come at the end of the document.
func itemChildren(d *parser.Document, i int) []int {
	var out []int
	for child := range d.Children(i) {
		if d.Block(child).Kind == parser.FootnoteDefinition {
			continue
		}
		out = append(out, child)
	}
	return out
}

// items returns the items of list i.
func items(d *parser.Document, i int) []int {
	var out []int
	for child := range d.Children(i) {
		if d.Block(child).Kind == parser.Item {
			out = append(out, child)
		}
	}
	return out
}

// indentLines prefixes each line that is not empty with indent, unless
// the block is a nested list, which sets its own indentation.
func indentLines(indent string, lines []string, nested bool) []string {
	if nested {
		return lines
	}
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = indentLine(indent, line)
	}
	return out
}

// indentLine prefixes a line, leaving empty lines empty.
func indentLine(prefix, line string) string {
	if line == "" {
		return ""
	}
	return prefix + line
}

// textLines splits rendered text into its lines.
func textLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

// rawLines splits a code, HTML or math block's lines, removing the final
// newline the parser adds to each line.
func rawLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

// finish removes trailing spaces from every line and blank lines from
// the ends of the document.
func finish(lines []string) string {
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " ")
	}
	start, end := 0, len(lines)
	for start < end && lines[start] == "" {
		start++
	}
	for end > start && lines[end-1] == "" {
		end--
	}
	return strings.Join(lines[start:end], "\n")
}
