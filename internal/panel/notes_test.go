package panel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/design"
)

// noteFiles reads the harness's notes directory, by file name.
func (h *harness) noteFiles() map[string]string {
	h.t.Helper()
	dir := h.p.notes.store.Dir
	entries, _ := os.ReadDir(dir)
	files := map[string]string{}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			h.t.Fatal(err)
		}
		files[e.Name()] = string(data)
	}
	return files
}

// enterNotes moves the harness's panel to notes mode from the first mode.
func (h *harness) enterNotes() {
	h.t.Helper()
	h.tt.Frame()
	h.tt.Click("原文")
	h.tt.Key(ui.Shift, ui.KeyTab)
	h.drain()
	if !h.p.noteMode() {
		h.t.Fatal("Shift-Tab from the first mode did not reach notes")
	}
}

func TestNoteIsSavedAsTyped(t *testing.T) {
	h := newHarness(t, script{})
	h.enterNotes()
	if !h.tt.Focused("笔记") {
		t.Fatal("the note does not have the keyboard")
	}
	h.tt.Type("# 买菜")
	h.tt.Key(0, ui.KeyEnter)
	h.tt.Type("- 鸡蛋 ")
	// A pasted link stays as it is: no Markdown link is made of it.
	h.tt.SetClipboard("https://example.com/a_b?c=1")
	h.tt.Key(ui.Cmd, ui.KeyV)
	h.drain()

	want := "# 买菜\n- 鸡蛋 https://example.com/a_b?c=1"
	if h.p.notes.text != want {
		t.Fatalf("note = %q, want %q", h.p.notes.text, want)
	}
	h.p.saveNote()
	files := h.noteFiles()
	if len(files) != 1 || files[h.p.notes.cur] != want {
		t.Fatalf("files = %v, want the note under %s", files, h.p.notes.cur)
	}
	h.snapshot("notes")

	// Leaving notes keeps the note, and coming back opens it again.
	h.tt.Key(0, ui.KeyTab)
	h.drain()
	if h.p.noteMode() {
		t.Fatal("Tab did not leave notes")
	}
	cur := h.p.notes.cur
	h.tt.Key(ui.Shift, ui.KeyTab)
	h.drain()
	if h.p.notes.cur != cur || h.p.notes.text != want {
		t.Errorf("back in notes: %s %q, want the note left", h.p.notes.cur, h.p.notes.text)
	}
}

func TestNewNoteAndList(t *testing.T) {
	h := newHarness(t, script{})
	h.enterNotes()
	h.tt.Type("买菜")
	h.tt.Key(ui.Cmd, ui.KeyN)
	h.drain()
	first := h.p.notes.all
	if len(first) != 1 || first[0].Text != "买菜" {
		t.Fatalf("Cmd-N did not save the note it left: %v", first)
	}
	h.tt.Type("会议纪要\n下周一提交")
	h.tt.Key(ui.Cmd, ui.KeyN)
	h.tt.Key(ui.Cmd, ui.KeyN) // a blank note gives no other
	h.drain()
	if n := len(h.noteFiles()); n != 2 {
		t.Fatalf("%d files, want 2: a blank note is not written", n)
	}

	h.tt.Key(ui.Cmd, ui.KeyP)
	h.drain()
	if !h.p.notes.listOpen || !h.tt.Focused("搜索笔记") {
		t.Fatal("Cmd-P did not open the list with the search focused")
	}
	h.snapshot("notes-browse")
	h.tt.Type("提交")
	h.drain()
	rows, create := h.p.listRows()
	if len(rows) != 1 || rows[0].Line != "下周一提交" || !create {
		t.Fatalf("search rows = %+v, create %v", rows, create)
	}
	h.snapshot("notes-list")
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	if h.p.notes.listOpen || !strings.HasPrefix(h.p.notes.text, "会议纪要") {
		t.Fatalf("Enter did not open the note found: %q", h.p.notes.text)
	}

	// What no note has makes a note of itself.
	h.tt.Key(ui.Cmd, ui.KeyP)
	h.tt.Type("周报")
	h.drain()
	h.tt.Key(0, ui.KeyEnter)
	h.drain()
	if h.p.notes.text != "周报" {
		t.Fatalf("note = %q, want one made of the search", h.p.notes.text)
	}
	if n := len(h.noteFiles()); n != 3 {
		t.Errorf("%d files, want 3", n)
	}
}

func TestStepAndDeleteNotes(t *testing.T) {
	h := newHarness(t, script{})
	h.enterNotes()
	for _, s := range []string{"一", "二", "三"} {
		h.tt.Type(s)
		h.tt.Key(ui.Cmd, ui.KeyN)
		h.drain()
	}
	h.tt.Key(ui.Cmd, ui.KeyBracketLeft)
	h.drain()
	if h.p.notes.text != "三" {
		t.Fatalf("Cmd-[ from a blank note: %q, want the latest", h.p.notes.text)
	}
	h.tt.Key(ui.Cmd, ui.KeyBracketLeft)
	h.tt.Key(ui.Cmd, ui.KeyBracketLeft)
	h.drain()
	if h.p.notes.text != "一" {
		t.Fatalf("two more Cmd-[: %q, want the oldest", h.p.notes.text)
	}
	h.tt.Key(ui.Cmd, ui.KeyBracketRight)
	h.drain()
	if h.p.notes.text != "二" {
		t.Fatalf("Cmd-]: %q, want the next", h.p.notes.text)
	}

	// The open note leads the list; deleting it opens the latest of the
	// others.
	h.tt.Key(ui.Cmd, ui.KeyP)
	h.drain()
	h.tt.Key(ui.Cmd, ui.KeyBackspace)
	h.drain()
	files := h.noteFiles()
	if len(files) != 2 || h.p.notes.text != "三" {
		t.Fatalf("after deleting: %v, open %q", files, h.p.notes.text)
	}

	// An emptied note leaves no file behind.
	h.tt.Key(0, ui.KeyEscape)
	h.drain()
	h.tt.Key(ui.Cmd, ui.KeyA)
	h.tt.Key(0, ui.KeyBackspace)
	h.tt.Key(0, ui.KeyTab)
	h.drain()
	if files := h.noteFiles(); len(files) != 1 {
		t.Errorf("after emptying a note: %v", files)
	}
}

func TestCmdClickOpensLink(t *testing.T) {
	h := newHarness(t, script{})
	var opened string
	h.p.openURL = func(url string) { opened = url }
	h.enterNotes()
	h.tt.Type("链接")
	h.tt.Key(0, ui.KeyEnter)
	h.tt.Type("看 https://example.com/x。")
	h.drain()
	// The editor is the first element labeled so, before the tab.
	r, ok := h.tt.Find("笔记")
	if !ok {
		t.Fatal("no note editor")
	}
	line := h.p.sz.Source * design.LineHeightSource
	h.tt.ClickAtWith(ui.Cmd, r.X+design.SpaceX+8*h.p.sz.Source, r.Y+design.SpaceSourceY+1.5*line)
	h.drain()
	if opened != "https://example.com/x" {
		t.Errorf("opened %q", opened)
	}
}

func TestListLeavesTheNoteAlone(t *testing.T) {
	h := newHarness(t, script{})
	h.enterNotes()
	h.tt.Type("第一张")
	h.tt.Key(ui.Cmd, ui.KeyN)
	h.tt.Type("第二张\n正文")
	h.drain()
	caret := h.p.notes.carets[h.p.notes.cur]
	height := h.p.panelH

	h.tt.Key(ui.Cmd, ui.KeyP)
	h.drain()
	if !h.tt.HasText("当前笔记") || !h.tt.HasText("正文") {
		t.Errorf("the open note is not marked, or no summary: %q", h.tt.Texts())
	}
	h.tt.Key(0, ui.KeyDown)
	h.tt.Frame()
	h.tt.Frame()
	if !strings.HasPrefix(h.p.notes.text, "第二张") || h.p.panelH != height {
		t.Errorf("choosing a row changed the note to %q or the panel from %v to %v", h.p.notes.text, height, h.p.panelH)
	}
	if m, _ := h.p.chosenNote(); m.Text != "第一张" || !h.tt.HasText("笔记预览") {
		t.Errorf("chose %q; the preview shows %v", m.Text, h.tt.HasText("笔记预览"))
	}
	h.snapshot("notes-preview")
	h.tt.Key(0, ui.KeyEscape)
	h.drain()
	if h.p.notes.listOpen || !h.tt.Focused("笔记") || h.p.notes.carets[h.p.notes.cur] != caret {
		t.Errorf("after Esc: open %v, note focused %v, caret %d want %d",
			h.p.notes.listOpen, h.tt.Focused("笔记"), h.p.notes.carets[h.p.notes.cur], caret)
	}

	h.tt.Key(ui.Cmd, ui.KeyP)
	h.drain()
	h.tt.Key(0, ui.KeyTab)
	h.drain()
	if h.p.notes.listOpen || h.p.noteMode() {
		t.Errorf("Tab in the list: open %v, still in notes %v", h.p.notes.listOpen, h.p.noteMode())
	}
}

func TestPreviewRightAfterOpeningANote(t *testing.T) {
	h := newHarness(t, script{})
	h.enterNotes()
	h.tt.Type("第一张")
	h.tt.Key(ui.Cmd, ui.KeyN)
	h.tt.Type("第二张")
	h.drain()
	height := h.p.panelH
	// Opening a note from the list makes its editor anew; the list opened
	// again at once must not take that editor's height before it is laid
	// out.
	check := func(when string) {
		t.Helper()
		h.tt.Frame()
		if h.p.panelH != height {
			t.Fatalf("%s: panel %v high, want %v", when, h.p.panelH, height)
		}
	}
	for range 3 {
		h.tt.Key(ui.Cmd, ui.KeyP)
		h.tt.Key(0, ui.KeyDown)
		h.tt.Key(0, ui.KeyEnter)
		h.tt.Key(ui.Cmd, ui.KeyP)
		h.tt.Key(0, ui.KeyDown)
		check("previewing right after opening a note")
		// Back on the open note, its editor is made anew, then away again.
		h.tt.Key(0, ui.KeyUp)
		h.tt.Key(0, ui.KeyDown)
		check("previewing right after the open note was chosen")
		// Before the editor was ever measured, a preview still has its
		// lines rather than none.
		h.p.notes.editorH = 0
		check("previewing before the editor was measured")
		h.tt.Key(0, ui.KeyEscape)
		h.tt.Frame()
	}
}

func TestListInAShortPanel(t *testing.T) {
	h := newHarness(t, script{})
	h.enterNotes()
	h.tt.Type("负载均衡流水线需求\n负载均衡有 3 个独立 worktree")
	h.tt.Key(ui.Cmd, ui.KeyN)
	h.tt.Type("PG接入需求\n四个仓库现在统一使用本地分支")
	h.tt.Key(ui.Cmd, ui.KeyN)
	h.tt.Type("docker tag registry.gz.cvte.cn/oma/managed-agent-sandbox:8ec80c34d892d68a64e908554c6ab2be38077d16 myou-registry.cvte.com/oma/managed-agent-sandbox:8ec80c34d892d68a64e908554c6ab2be38077d16")
	h.drain()
	h.p.fixedHeight = 257
	h.tt.Key(ui.Cmd, ui.KeyP)
	h.drain()
	h.tt.Frame()
	dot, ok := h.tt.Find("当前笔记")
	if !ok {
		t.Fatal("no dot on the open note")
	}
	when, _ := h.tt.Find("刚刚") // of the first row
	if dot.X+dot.W > when.X || dot.Y > when.Y+when.H || when.Y < dot.Y-when.H {
		t.Errorf("the dot is at %v, the time at %v: a long title pushed them apart", dot, when)
	}
	h.snapshot("notes-list-short")
}

func TestDayGroup(t *testing.T) {
	now := time.Date(2026, 10, 22, 9, 30, 0, 0, time.Local)
	for _, c := range []struct {
		t          time.Time
		group, row string
	}{
		{now.Add(-time.Minute * 5), "今天", "5 分钟前"},
		{time.Date(2026, 10, 22, 0, 10, 0, 0, time.Local), "今天", "00:10"},
		{time.Date(2026, 10, 21, 23, 50, 0, 0, time.Local), "昨天", "23:50"},
		{time.Date(2026, 10, 16, 8, 0, 0, 0, time.Local), "最近 7 天", "10月16日"},
		{time.Date(2026, 10, 15, 8, 0, 0, 0, time.Local), "更早", "10月15日"},
	} {
		if g, r := dayGroup(c.t, now), rowTime(c.t, now); g != c.group || r != c.row {
			t.Errorf("%v: %s, %s; want %s, %s", c.t, g, r, c.group, c.row)
		}
	}
}
