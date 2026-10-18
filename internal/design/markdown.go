package design

import (
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
)

// Markdown returns how replies look in a palette: monocode's measures in
// the theme's colors and the one font.
func Markdown(p *Palette, font string) *markdown.Theme {
	return &markdown.Theme{
		FontSize:       FontBody,
		Font:           font,
		CodeFont:       font,
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
