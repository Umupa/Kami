package plaintext

import (
	"strings"
	"testing"
)

func TestFromMarkdown(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "empty",
			in:   "",
			want: "",
		},
		{
			name: "blank lines",
			in:   "\n \n\t\n",
			want: "",
		},
		{
			name: "headings",
			in:   "# Title\n\n## Second *level*\n\n###### Sixth",
			want: "Title\n\nSecond level\n\nSixth",
		},
		{
			name: "soft breaks in Latin text",
			in:   "one\ntwo\nthree",
			want: "one two three",
		},
		{
			name: "soft breaks in Chinese",
			in:   "中文\n继续",
			want: "中文继续",
		},
		{
			name: "soft breaks mixing Chinese and English",
			in:   "他说\nhello\n世界",
			want: "他说 hello 世界",
		},
		{
			name: "hard breaks",
			in:   "one  \ntwo\\\nthree",
			want: "one\ntwo\nthree",
		},
		{
			name: "emphasis and inline HTML",
			in:   "**bold** *em* ~~del~~ `code` <b>tag</b>",
			want: "bold em del code tag",
		},
		{
			name: "link",
			in:   "[OpenAI](https://openai.com)",
			want: "OpenAI (https://openai.com)",
		},
		{
			name: "autolink",
			in:   "<https://example.com>",
			want: "https://example.com",
		},
		{
			name: "bare autolink",
			in:   "https://example.com",
			want: "https://example.com",
		},
		{
			name: "www autolink",
			in:   "www.example.com",
			want: "www.example.com",
		},
		{
			name: "email autolink",
			in:   "<a@example.com>",
			want: "a@example.com",
		},
		{
			name: "link whose text is its URL",
			in:   "[https://example.com](https://example.com)",
			want: "https://example.com",
		},
		{
			name: "image",
			in:   "![A *cat*](cat.png)",
			want: "A cat",
		},
		{
			name: "unordered list",
			in:   "- one\n- two",
			want: "• one\n• two",
		},
		{
			name: "ordered list starts at its number",
			in:   "3. three\n4. four",
			want: "3. three\n4. four",
		},
		{
			name: "task list",
			in:   "- [ ] todo\n- [x] done",
			want: "☐ todo\n☑ done",
		},
		{
			name: "nested lists",
			in:   "- one\n  - two\n    - three\n- four",
			want: "• one\n  • two\n    • three\n• four",
		},
		{
			name: "ordered nested list",
			in:   "1. one\n   1. two\n2. three",
			want: "1. one\n  1. two\n2. three",
		},
		{
			name: "loose list",
			in:   "- one\n- two\n\n- three",
			want: "• one\n\n• two\n\n• three",
		},
		{
			name: "continuation paragraph in item",
			in:   "- one\n\n  two",
			want: "• one\n\n  two",
		},
		{
			name: "continuation paragraph in ordered item",
			in:   "1. one\n\n   two",
			want: "1. one\n\n   two",
		},
		{
			name: "hard break in item",
			in:   "- one  \n  two",
			want: "• one\n  two",
		},
		{
			name: "block quote",
			in:   "> quoted\n>\n> second",
			want: "quoted\n\nsecond",
		},
		{
			name: "code block",
			in:   "```go\nfmt.Println(\"hi\")\n\nif true {\n}\n```",
			want: "fmt.Println(\"hi\")\n\nif true {\n}",
		},
		{
			name: "code block in item",
			in:   "- run:\n  ```\n  go test\n  ```",
			want: "• run:\n  go test",
		},
		{
			name: "table",
			in:   "| Name | Value |\n| :--- | ---: |\n| a | 1 |\n| b | 2 |",
			want: "Name\tValue\na\t1\nb\t2",
		},
		{
			name: "table with escaped pipe",
			in:   "| a | b |\n| --- | --- |\n| x \\| y | z |",
			want: "a\tb\nx | y\tz",
		},
		{
			name: "table with empty cells",
			in:   "| | |\n| --- | --- |\n| | |",
			want: "\t\n\t",
		},
		{
			name: "thematic break is omitted",
			in:   "one\n\n---\n\ntwo\n\n***",
			want: "one\n\ntwo",
		},
		{
			name: "math",
			in:   "Inline $$x^2$$ math.\n\n$$\nx + y\n$$",
			want: "Inline x^2 math.\n\nx + y",
		},
		{
			name: "bracket math",
			in:   "Inline \\(a+b\\) and display \\[c\\] math.",
			want: "Inline a+b and display c math.",
		},
		{
			name: "HTML block",
			in:   "<div>\nraw &amp;\n</div>",
			want: "<div>\nraw &amp;\n</div>",
		},
		{
			name: "footnotes",
			in:   "Text[^1] and[^long].\n\n[^1]: First note.\n[^long]: Second note.",
			want: "Text[1] and[2].\n\n[1] First note.\n\n[2] Second note.",
		},
		{
			name: "entities and escapes",
			in:   "AT&amp;T \\*not em\\* &lt;tag&gt;",
			want: "AT&T *not em* <tag>",
		},
		{
			name: "trailing spaces and blank edges",
			in:   "\n\n# H  \n\npara   \n\n```\ncode   \n```\n\n",
			want: "H\n\npara\n\ncode",
		},
		{
			name: "typical model reply",
			in: "## Result\n\n" +
				"The fix is **simple**: keep the source.\n\n" +
				"- Read the input\n" +
				"  - normalize it\n" +
				"  - validate it\n" +
				"- Write the output\n\n" +
				"```go\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n```\n\n" +
				"| Step | Command |\n" +
				"| --- | --- |\n" +
				"| build | go build |\n" +
				"| test | go test ./... |",
			want: "Result\n\n" +
				"The fix is simple: keep the source.\n\n" +
				"• Read the input\n" +
				"  • normalize it\n" +
				"  • validate it\n" +
				"• Write the output\n\n" +
				"func main() {\n\tfmt.Println(\"hi\")\n}\n\n" +
				"Step\tCommand\n" +
				"build\tgo build\n" +
				"test\tgo test ./...",
		},
		{
			name: "unclosed code fence",
			in:   "Here is the code:\n\n```go\nfunc main() {\n",
			want: "Here is the code:\n\nfunc main() {",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromMarkdown(tt.in)
			if got != tt.want {
				t.Errorf("FromMarkdown(%q) =\n%q\nwant\n%q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFromMarkdownStreaming(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "unclosed code fence",
			in:   "Here:\n\n```go\nfmt.Println(\"hi\")\n",
			want: "Here:\n\nfmt.Println(\"hi\")",
		},
		{
			name: "half a table",
			in:   "| Name | Value |\n| --- |",
			want: "| Name | Value | | --- |",
		},
		{
			name: "table header without delimiter",
			in:   "| Name | Value |\n",
			want: "| Name | Value |",
		},
		{
			name: "unclosed emphasis",
			in:   "Working on **the fix",
			want: "Working on **the fix",
		},
		{
			name: "unclosed link",
			in:   "See [the docs",
			want: "See [the docs",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FromMarkdown(tt.in)
			if got != tt.want {
				t.Errorf("FromMarkdown(%q) =\n%q\nwant\n%q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFromMarkdownNeverPanics(t *testing.T) {
	inputs := []string{
		"",
		"#",
		"```",
		"|",
		"| a |\n| - |\n|",
		"[^1]:",
		"[^1]: note\n\nx[^1]",
		"$$",
		"\\(",
		"<div>",
		"- ",
		"1. ",
		"> ",
	}
	for _, in := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("FromMarkdown(%q) panicked: %v", in, r)
				}
			}()
			_ = FromMarkdown(in)
		}()
	}
}

func TestFromMarkdownEndsCleanly(t *testing.T) {
	in := "\n\n```\n  code  \n\n\n```\n\n---\n\n\n"
	got := FromMarkdown(in)
	if strings.HasPrefix(got, "\n") || strings.HasSuffix(got, "\n") {
		t.Errorf("FromMarkdown(%q) has a blank line at an end: %q", in, got)
	}
	if strings.Contains(got, "code  ") {
		t.Errorf("FromMarkdown(%q) kept trailing spaces: %q", in, got)
	}
}
