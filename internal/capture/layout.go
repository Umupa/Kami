// Portions adapted from Xuanwo/cida, Sources/Cida/TextRecognition.swift,
// commit 3099e0dcbc2bafe0a86e6a04d1941b78afacbfd6 (Swift to Go).
// Licensed under Apache-2.0; see third_party/cida/LICENSE.
package capture

import (
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Umupa/Kami/internal/macos"
)

// recognizedText preserves intentional breaks while rejoining visual wraps.
func recognizedText(lines []macos.Line) string {
	clean := make([]macos.Line, 0, len(lines))
	for _, l := range lines {
		if l.Text = strings.TrimSpace(l.Text); l.Text != "" {
			clean = append(clean, l)
		}
	}
	if len(clean) == 0 {
		return ""
	}
	sort.SliceStable(clean, func(i, j int) bool { return clean[i].Box.Y+clean[i].Box.H/2 < clean[j].Box.Y+clean[j].Box.H/2 })
	var groups [][]macos.Line
	for _, l := range clean {
		if len(groups) > 0 {
			last := groups[len(groups)-1]
			b := last[len(last)-1].Box
			if math.Abs(l.Box.Y+l.Box.H/2-b.Y-b.H/2) < min(l.Box.H, b.H)/2 {
				groups[len(groups)-1] = append(last, l)
				continue
			}
		}
		groups = append(groups, []macos.Line{l})
	}
	rows := make([]macos.Line, 0, len(groups))
	for _, group := range groups {
		sort.SliceStable(group, func(i, j int) bool { return group[i].Box.X < group[j].Box.X })
		row := group[0]
		for _, l := range group[1:] {
			row.Text += " " + l.Text
			x, y := min(row.Box.X, l.Box.X), min(row.Box.Y, l.Box.Y)
			row.Box = macos.Rect{X: x, Y: y, W: max(row.Box.X+row.Box.W, l.Box.X+l.Box.W) - x, H: max(row.Box.Y+row.Box.H, l.Box.Y+l.Box.H) - y}
		}
		rows = append(rows, row)
	}
	heights := make([]float64, 0, len(rows))
	right := float64(0)
	for _, r := range rows {
		heights = append(heights, r.Box.H)
		right = max(right, r.Box.X+r.Box.W)
	}
	sort.Float64s(heights)
	typical := heights[len(heights)/2]
	text := rows[0].Text
	for i, row := range rows[1:] {
		previous := rows[i]
		if row.Box.Y-previous.Box.Y-previous.Box.H > typical*0.8 || startsListItem(row.Text) || roomForWord(previous, row, right) {
			text += "\n" + row.Text
		} else {
			text = joinWrapped(text, row.Text)
		}
	}
	return strings.TrimSpace(text)
}

func roomForWord(previous, row macos.Line, right float64) bool {
	first, _ := utf8.DecodeRuneInString(row.Text)
	word := 0
	if isCJK(first) {
		word = 1
	} else {
		for _, r := range row.Text {
			if unicode.IsSpace(r) {
				break
			}
			word++
		}
	}
	characterWidth := previous.Box.W / float64(utf8.RuneCountInString(previous.Text))
	return right-previous.Box.X-previous.Box.W > characterWidth*(float64(word)*1.4+1)
}

func startsListItem(text string) bool {
	first, n := utf8.DecodeRuneInString(text)
	if strings.ContainsRune("•◦▪▫‣∙·●○■□▸►", first) {
		return true
	}
	if strings.ContainsRune("-*+", first) {
		return strings.HasPrefix(text[n:], " ")
	}
	digits := 0
	for digits < len(text) && text[digits] >= '0' && text[digits] <= '9' {
		digits++
	}
	if digits < 1 || digits > 3 {
		return false
	}
	marker := text[digits:]
	return strings.HasPrefix(marker, "、") || strings.HasPrefix(marker, ". ") || strings.HasPrefix(marker, ") ")
}

func joinWrapped(head, tail string) string {
	last, n := utf8.DecodeLastRuneInString(head)
	first, _ := utf8.DecodeRuneInString(tail)
	if head == "" || tail == "" || isCJK(last) || isCJK(first) {
		return head + tail
	}
	before, _ := utf8.DecodeLastRuneInString(head[:len(head)-n])
	if last == '-' && unicode.IsLetter(before) && unicode.IsLower(first) {
		return head[:len(head)-n] + tail
	}
	return head + " " + tail
}

func isCJK(r rune) bool {
	return r >= 0x3000 && r <= 0x30ff || r >= 0x3400 && r <= 0x4dbf || r >= 0x4e00 && r <= 0x9fff || r >= 0xf900 && r <= 0xfaff || r >= 0xff00 && r <= 0xffef
}
