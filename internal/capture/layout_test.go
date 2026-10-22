package capture

import (
	"testing"

	"github.com/Umupa/Kami/internal/macos"
)

func TestRecognizedParagraphsMatchCida(t *testing.T) {
	line := func(text string, row int, width float64) macos.Line {
		return macos.Line{Text: text, Box: macos.Rect{X: 0.1, Y: 0.1 + float64(row)*0.06, W: width, H: 0.04}}
	}
	tests := []struct {
		name  string
		lines []macos.Line
		want  string
	}{
		{"blank", []macos.Line{{Text: "  "}}, ""},
		{"wrapped", []macos.Line{line("The new storage engine keeps every", 0, .48), line("write in an append-only log and com-", 1, .48), line("pacts it in the background.", 2, .36), line("我们的系统采用了全新的存储", 4, .48), line("引擎，显著提升了读写性能。", 5, .40)}, "The new storage engine keeps every write in an append-only log and compacts it in the background.\n我们的系统采用了全新的存储引擎，显著提升了读写性能。"},
		{"intentional breaks", []macos.Line{line("明天的评审改到下午三点。", 0, .24), line("我们的系统采用了全新的存储引擎，显著提升了读写性", 1, .48), line("能，也降低了成本。", 2, .18), line("1. 先合并存储引擎的改动", 3, .26), line("2. 再发布新版本", 4, .16)}, "明天的评审改到下午三点。\n我们的系统采用了全新的存储引擎，显著提升了读写性能，也降低了成本。\n1. 先合并存储引擎的改动\n2. 再发布新版本"},
		{"same row", []macos.Line{{Text: "改进", Box: macos.Rect{X: .3, Y: .1, W: .1, H: .04}}, {Text: "翻译", Box: macos.Rect{X: .1, Y: .1, W: .1, H: .04}}}, "翻译 改进"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := recognizedText(tt.lines); got != tt.want {
				t.Fatalf("text = %q, want %q", got, tt.want)
			}
		})
	}
}
