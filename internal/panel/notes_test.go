package panel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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
