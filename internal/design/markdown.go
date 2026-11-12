package design

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
)

// Markdown returns how replies look in a palette: monocode's measures in
// the theme's colors and the one font.
func Markdown(p *Palette, size float32) *markdown.Theme {
	return &markdown.Theme{
		FontSize:       size,
		Font:           FontFamily,
		CodeFont:       FontFamily,
		LineHeight:     LineHeightBody,
		CodeLineHeight: 1.5,

		Text:         p.Text,
		Muted:        p.Muted,
		Link:         p.Link,
		Strong:       p.Bright,
		StrongWeight: 700,

		Code:           p.Code,
		CodeBackground: p.Surface,
		CodeSize:       0.92,

		BlockSpacing: 0.9,
		ItemSpacing:  0.3,
		Headings: [6]markdown.HeadingStyle{
			{Size: 1.35, Weight: 700, Color: p.Accent, Top: 1.2, Bottom: 0.5},
			{Size: 1.18, Weight: 700, Color: p.Accent, Top: 1.1, Bottom: 0.45},
			{Size: 1.05, Weight: 700, Color: p.Bright, Top: 1, Bottom: 0.4},
			{Size: 1, Weight: 700, Color: p.Bright, Top: 0.9, Bottom: 0.35},
			{Size: 1, Weight: 600, Color: p.Muted},
			{Size: 1, Weight: 600, Color: p.Muted},
		},
		ListIndent: 1.6,

		QuoteBar:      p.Quote,
		QuoteText:     p.Muted,
		QuoteBarWidth: 2,
		Rule:          p.Border,

		CodeBlockBackground: p.Surface,
		CodeBlockBorder:     p.Border,
		CodeBlockRadius:     RadiusBlock,
		CodeBlockPadding:    0.9,
		CodeColors:          Syntax(p),

		TableBorder:      p.Border,
		TableHeader:      p.Surface,
		TableCellPadding: [2]float32{0.75, 0.5},
		TableMaxColumn:   28,

		Mark:     p.Sparkle.Alpha(0.3),
		Checkbox: p.Accent,
	}
}

// Syntax colors code in a palette.
func Syntax(p *Palette) *editor.Theme {
	t := editor.DefaultTheme(&ui.Theme{Dark: p.Dark})
	t.Background, t.Text = p.Surface, p.Bright
	set := func(k editor.TokenKind, c ui.Color) { t.Tokens[k] = editor.TokenStyle{Color: c} }
	set(editor.TokenKeyword, p.Keyword)
	set(editor.TokenType, p.Type)
	set(editor.TokenFunction, p.Function)
	set(editor.TokenString, p.String)
	set(editor.TokenNumber, p.Number)
	set(editor.TokenConstant, p.Number)
	set(editor.TokenComment, p.Comment)
	set(editor.TokenProperty, p.Link)
	set(editor.TokenOperator, p.Link)
	set(editor.TokenPunctuation, p.Muted)
	set(editor.TokenHeading, p.Accent)
	set(editor.TokenLink, p.Link)
	set(editor.TokenInserted, p.String)
	set(editor.TokenDeleted, p.Danger)
	return t
}

// FoldCode builds code blocks of more than lines lines folded to their
// first lines, fading into the block, with a toggle below to show them
// whole. It is for text handed on rather than read; copying a block still
// copies all of it.
func FoldCode(th *markdown.Theme, lines int) func(c *ui.Context, b *markdown.CodeBlockView) {
	return func(c *ui.Context, b *markdown.CodeBlockView) {
		if strings.Count(strings.TrimRight(b.Code(), "\n"), "\n")+1 <= lines+1 {
			markdown.DefaultCodeBlock(c, b)
			return
		}
		col := ui.Column(c).AlignSelf(ui.Stretch).Gap(th.FontSize * 0.35)
		open := ui.Local(col, "open", func() bool { return false })
		col.Children(func() {
			em := func(v float32) float32 { return v * th.FontSize }
			box := ui.Column(c).Background(th.CodeBlockBackground).Border(1, th.CodeBlockBorder).Radius(th.CodeBlockRadius).AlignSelf(ui.Stretch).Clip()
			box.Children(func() {
				ui.Row(c).AlignItems(ui.Center).Padding(em(0.35), em(0.4), 0, em(th.CodeBlockPadding)).Children(func() {
					ui.Text(c, b.Language).FontSize(em(0.8)).TextColor(th.Muted).Unselectable()
					ui.Spacer(c)
					b.CopyButton(c)
				})
				if *open {
					b.Body(c)
					return
				}
				// The top padding of the body and the lines shown.
				line := em(th.CodeSize) * th.CodeLineHeight
				body := ui.Column(c).AlignSelf(ui.Stretch).Height(em(0.25) + float32(lines)*line).Clip()
				body.Children(func() {
					b.Body(c)
					ui.Box(c).PassThrough().Absolute().Left(0).Right(0).Bottom(0).Height(2*line).
						Gradient(th.CodeBlockBackground.Alpha(0), th.CodeBlockBackground, 180)
				})
			})
			label := "展开"
			if *open {
				label = "收起"
			}
			// No side padding: the label lines up with the text and the block.
			btn := ui.ButtonBase(c).AlignSelf(ui.Start).Padding(em(0.1), 0).Label(label)
			color := th.Muted
			if btn.Hovered() {
				color = th.Text
			}
			btn.Children(func() {
				ui.Text(c, label).FontSize(em(0.85)).TextColor(color)
			})
			if btn.Clicked() {
				*open = !*open
			}
		})
	}
}

// FoldLines is how many lines a folded code block shows.
const FoldLines = 4
