package segment

import (
	"math/rand/v2"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitBoundaryKinds(t *testing.T) {
	tests := []struct {
		name string
		text string
		a    string
		b    string
	}{
		{
			name: "blank line",
			text: "First paragraph.\n\nSecond paragraph.",
			a:    "First paragraph.\n\n",
			b:    "Second paragraph.",
		},
		{
			name: "line break",
			text: "line one\nline two",
			a:    "line one\n",
			b:    "line two",
		},
		{
			name: "sentence",
			text: "Hello. World",
			a:    "Hello. ",
			b:    "World",
		},
		{
			name: "Chinese sentence",
			text: "你好。世界",
			a:    "你好。",
			b:    "世界",
		},
		{
			name: "ellipsis",
			text: "Wait… What",
			a:    "Wait… ",
			b:    "What",
		},
		{
			name: "whitespace",
			text: "hello world",
			a:    "hello ",
			b:    "world",
		},
		{
			name: "characters",
			text: "你好世界",
			a:    "你好",
			b:    "世界",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b, ok := Split(tt.text)
			if !ok {
				t.Fatalf("Split(%q) ok = false", tt.text)
			}
			if a != tt.a || b != tt.b {
				t.Errorf("Split(%q) = %q, %q, want %q, %q", tt.text, a, b, tt.a, tt.b)
			}
			if a+b != tt.text {
				t.Errorf("Split(%q) lost bytes: %q + %q", tt.text, a, b)
			}
		})
	}
}

func TestSplitPreference(t *testing.T) {
	tests := []struct {
		name string
		text string
		a    string
		b    string
	}{
		{
			name: "blank line beats a closer sentence",
			text: "A. B\n\nC",
			a:    "A. B\n\n",
			b:    "C",
		},
		{
			name: "line break beats a closer sentence",
			text: "Hello. World\nNext",
			a:    "Hello. World\n",
			b:    "Next",
		},
		{
			name: "sentence beats closer whitespace",
			text: "你好。世界 和平",
			a:    "你好。",
			b:    "世界 和平",
		},
		{
			name: "whitespace beats closer character boundary",
			text: "abc def ghi",
			a:    "abc ",
			b:    "def ghi",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b, ok := Split(tt.text)
			if !ok {
				t.Fatalf("Split(%q) ok = false", tt.text)
			}
			if a != tt.a || b != tt.b {
				t.Errorf("Split(%q) = %q, %q, want %q, %q", tt.text, a, b, tt.a, tt.b)
			}
		})
	}
}

func TestSplitMiddle80(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{
			name: "blank line outside the middle is skipped",
			text: "a\n\n" + strings.Repeat("x", 40) + "\n" + strings.Repeat("y", 40),
			want: "a\n\n" + strings.Repeat("x", 40) + "\n",
		},
		{
			name: "sentence outside the middle is skipped",
			text: "Done. " + strings.Repeat("x", 40) + " " + strings.Repeat("y", 40),
			want: "Done. " + strings.Repeat("x", 40) + " ",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b, ok := Split(tt.text)
			if !ok {
				t.Fatalf("Split(%q) ok = false", tt.text)
			}
			if a != tt.want {
				t.Errorf("a = %q, want %q (b = %q)", a, tt.want, b)
			}
			if a+b != tt.text {
				t.Errorf("Split(%q) lost bytes", tt.text)
			}
		})
	}
}

func TestSplitTinyAndCRLF(t *testing.T) {
	tests := []struct {
		name string
		text string
		a    string
		b    string
		ok   bool
	}{
		{name: "empty", text: ""},
		{name: "one ASCII character", text: "a"},
		{name: "one multibyte character", text: "中"},
		{name: "one emoji", text: "😀"},
		{name: "two ASCII characters", text: "ab", a: "a", b: "b", ok: true},
		{name: "two Han characters", text: "你好", a: "你", b: "好", ok: true},
		{name: "only a CRLF pair", text: "\r\n"},
		{name: "CRLF then text", text: "\r\na", a: "\r\n", b: "a", ok: true},
		{name: "text then CRLF", text: "a\r\n", a: "a", b: "\r\n", ok: true},
		{name: "CRLF in the middle", text: "ab\r\ncd", a: "ab\r\n", b: "cd", ok: true},
		{name: "lone CR", text: "a\rb", a: "a\r", b: "b", ok: true},
		{name: "two CRLF pairs", text: "\r\n\r\n", a: "\r\n", b: "\r\n", ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b, ok := Split(tt.text)
			if ok != tt.ok || a != tt.a || b != tt.b {
				t.Errorf("Split(%q) = %q, %q, %v, want %q, %q, %v", tt.text, a, b, ok, tt.a, tt.b, tt.ok)
			}
		})
	}
}

func TestSplitKeepsEveryByte(t *testing.T) {
	corpus := []string{
		"",
		"a",
		"ab",
		"abc",
		"你好",
		"你好世界",
		"这是一段没有空格的中文文本，需要按字符切分。",
		"Hello. World",
		"First paragraph.\n\nSecond paragraph.",
		"line one\nline two",
		"hello world",
		"真的吗？！然后",
		"Wait… What",
		"emoji 😀 test",
		"a\r\nb",
		"\r\n\r\n",
		"a\rb",
		"  leading and trailing  ",
		"Done. " + strings.Repeat("x", 100) + " " + strings.Repeat("y", 100),
		"a\n\n" + strings.Repeat("x", 40) + "\n" + strings.Repeat("y", 40),
	}
	alphabet := []string{
		"a", "b", "c", " ", "\t", "\n", "\r", ".", "!", "?", "…",
		"中", "你", "。", "！", "？", "😀",
	}
	rng := rand.New(rand.NewPCG(1, 2))
	for i := 0; i < 500; i++ {
		var b strings.Builder
		for n := rng.IntN(50); n > 0; n-- {
			b.WriteString(alphabet[rng.IntN(len(alphabet))])
		}
		corpus = append(corpus, b.String())
	}

	for _, text := range corpus {
		a, b, ok := Split(text)
		if !ok {
			if utf8.RuneCountInString(text) >= 2 && text != "\r\n" {
				t.Fatalf("Split(%q) ok = false for at least two characters", text)
			}
			continue
		}
		if a == "" || b == "" {
			t.Fatalf("Split(%q) returned an empty part: %q, %q", text, a, b)
		}
		if a+b != text {
			t.Fatalf("Split(%q) lost bytes: %q + %q", text, a, b)
		}
		if !utf8.ValidString(a) || !utf8.ValidString(b) {
			t.Fatalf("Split(%q) broke UTF-8: %q, %q", text, a, b)
		}
		if strings.HasSuffix(a, "\r") && strings.HasPrefix(b, "\n") {
			t.Fatalf("Split(%q) split a CRLF pair: %q, %q", text, a, b)
		}
		pos, n := len(a), len(text)
		if pos*10 < n || pos*10 > 9*n {
			t.Fatalf("Split(%q) boundary %d is outside the middle 80%% of %d bytes", text, pos, n)
		}
	}
}
