package note

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestTitle(t *testing.T) {
	for _, tc := range []struct{ text, want string }{
		{"", ""},
		{"\n\n  \n", ""},
		{"买菜\n鸡蛋", "买菜"},
		{"\n  ## 会议纪要  \n", "会议纪要"},
		{"> 引用的一句", "引用的一句"},
		{"- [ ] 待办", "待办"},
		{"* 列表", "列表"},
		{"#\n正文", "正文"},
	} {
		if got := Title(tc.text); got != tc.want {
			t.Errorf("Title(%q) = %q, want %q", tc.text, got, tc.want)
		}
	}
}

func TestStore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "notes")
	s := NewStore(dir, nil)
	s.now = func() time.Time { return time.Date(2026, 10, 19, 14, 30, 12, 0, time.Local) }

	if list, err := s.List(); err != nil || len(list) != 0 {
		t.Fatalf("List of no directory = %v, %v", list, err)
	}
	a := s.NewName(func(string) bool { return false })
	if a != "2026-10-19-143012.md" {
		t.Fatalf("NewName = %s", a)
	}
	if _, err := s.Write(a, "一"); err != nil {
		t.Fatal(err)
	}
	b := s.NewName(func(string) bool { return false })
	if b != "2026-10-19-143012-2.md" {
		t.Fatalf("NewName beside a file = %s", b)
	}
	if c := s.NewName(func(n string) bool { return n == b }); c != "2026-10-19-143012-3.md" {
		t.Fatalf("NewName beside a taken name = %s", c)
	}
	if _, err := s.Write(b, "二"); err != nil {
		t.Fatal(err)
	}
	// Only notes are listed: no hidden files, no folders, no other files.
	os.WriteFile(filepath.Join(dir, ".tmp-1"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("x"), 0o644)
	os.Mkdir(filepath.Join(dir, "sub.md"), 0o755)
	old := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(dir, b), old, old)

	list, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, n := range list {
		names = append(names, n.Name)
	}
	if !slices.Equal(names, []string{a, b}) {
		t.Fatalf("List = %v, want the latest edited first", names)
	}
	if list[0].Text != "一" {
		t.Errorf("text = %q", list[0].Text)
	}

	if err := s.Delete(a); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(a); err != nil {
		t.Errorf("Remove of a note gone = %v, want none", err)
	}
	if list, _ := s.List(); len(list) != 1 {
		t.Errorf("after Delete: %v", list)
	}
}

func TestDeleteGoesToTrash(t *testing.T) {
	var trashed string
	s := NewStore(t.TempDir(), func(path string) error { trashed = path; return nil })
	if err := s.Delete("a.md"); err != nil {
		t.Fatal(err)
	}
	if trashed != filepath.Join(s.Dir, "a.md") {
		t.Errorf("trashed %q", trashed)
	}
}

func TestSearch(t *testing.T) {
	notes := []Note{
		{Name: "1", Text: "会议纪要\n下周一提交 Report"},
		{Name: "2", Text: "Report 草稿"},
		{Name: "3", Text: "买菜"},
	}
	if got := Search(notes, "  "); len(got) != 3 {
		t.Errorf("blank search = %d notes, want all", len(got))
	}
	got := Search(notes, "report")
	if len(got) != 2 {
		t.Fatalf("search = %+v", got)
	}
	if got[0].Name != "1" || got[0].Line != "下周一提交 Report" || !slices.Equal(got[0].Spans, [][2]int{{6, 12}}) {
		t.Errorf("a match in the text = %+v", got[0])
	}
	if got[1].Name != "2" || got[1].Line != "" || !slices.Equal(got[1].Title, [][2]int{{0, 6}}) {
		t.Errorf("a match in the title = %+v", got[1])
	}
	if got := Search(notes, "没有"); len(got) != 0 {
		t.Errorf("search for nothing = %+v", got)
	}
}

func TestHighlight(t *testing.T) {
	text := "# 标题\n- [ ] 待办 **粗** `code`\n> 引用\n```\nx := 1\n```\n看 https://a.b/c。"
	want := []Span{
		{0, 4, Heading}, {0, 1, Mark},
		{5, 10, Mark}, {14, 19, Strong}, {20, 26, Code},
		{27, 31, Quote}, {27, 28, Mark},
		{32, 35, Code}, {36, 42, Code}, {43, 46, Code},
		{49, 62, Link},
	}
	if got := Highlight(text); !slices.Equal(got, want) {
		t.Errorf("Highlight =\n%v\nwant\n%v", got, want)
	}
}

func TestURLAt(t *testing.T) {
	text := "第一行\n看 https://example.com/a_b?c=1，然后"
	for _, tc := range []struct {
		at   int
		want string
	}{
		{0, ""},
		{6, "https://example.com/a_b?c=1"},
		{20, "https://example.com/a_b?c=1"},
		{33, "https://example.com/a_b?c=1"},
		{35, ""},
	} {
		if got := URLAt(text, tc.at); got != tc.want {
			t.Errorf("URLAt(%d) = %q, want %q", tc.at, got, tc.want)
		}
	}
}

func TestSummary(t *testing.T) {
	for text, want := range map[string]string{
		"# 周报\n\n  下周一提交  \n其他": "下周一提交",
		"只有标题":        "",
		"\n\n标题\n第二行": "第二行",
		"#\n标题\n正文":   "正文",
		"":            "",
	} {
		if got := Summary(text); got != want {
			t.Errorf("Summary(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestImages(t *testing.T) {
	s := NewStore(t.TempDir(), nil)
	for _, name := range []string{"2026-10-19-143012.md", "2026-10-19-143012-2.md"} {
		for range 2 {
			if _, err := s.SaveImage(name, []byte("png"), ".PNG"); err != nil {
				t.Fatal(err)
			}
		}
	}
	path, _ := s.SaveImage("2026-10-19-143012.md", []byte("png"), ".png")
	if path != "assets/2026-10-19-143012-img3.png" {
		t.Errorf("path = %q", path)
	}
	if got := len(s.Images("2026-10-19-143012.md")); got != 3 {
		t.Errorf("images of the first note = %d, want 3, not its neighbor's", got)
	}
	if _, err := s.SaveImage("a.md", make([]byte, MaxImage+1), ".png"); err != ErrImageTooLarge {
		t.Errorf("large image: %v", err)
	}
	if err := s.Remove("2026-10-19-143012-2.md"); err != nil {
		t.Fatal(err)
	}
	if len(s.Images("2026-10-19-143012-2.md")) != 0 || len(s.Images("2026-10-19-143012.md")) != 3 {
		t.Error("removing a note did not take only its images")
	}
}

func TestImageLink(t *testing.T) {
	text := "![](assets/a-img1.png)\n看 ![图](assets/b-img2.png) 这里"
	if got := Title(text); got != "[图片]" {
		t.Errorf("Title = %q", got)
	}
	if got := Summary(text); got != "看 [图片] 这里" {
		t.Errorf("Summary = %q", got)
	}
	if got := ImageAt(text, 5); got != "assets/a-img1.png" {
		t.Errorf("ImageAt = %q", got)
	}
	if got := ImageAt(text, 26); got != "assets/b-img2.png" {
		t.Errorf("ImageAt = %q", got)
	}
	if got := ImageAt(text, 24); got != "" {
		t.Errorf("ImageAt before the link = %q", got)
	}
	var link bool
	for _, sp := range Highlight(text) {
		link = link || (sp.Kind == Link && sp.Start == 0 && sp.End == 22)
	}
	if !link {
		t.Error("the image's Markdown is not colored as a link")
	}
}
