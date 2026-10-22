package panel

import (
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"

	"github.com/Umupa/Kami/internal/design"
	"github.com/Umupa/Kami/internal/note"
)

// Notes mode is the last tab of the mode bar: scratch notes, saved as they
// are typed, with no model behind them. Its state lives as long as the app.
type notes struct {
	store *note.Store
	// all are the notes as last listed and written, the most recently
	// edited first; loaded once the first listing came back.
	all    []note.Note
	loaded bool

	// cur is the open note's file name, "" before one is open. text is
	// what the editor holds, saved what the file holds, and exists whether
	// there is a file yet: a new note has none until it has text.
	cur, text, saved string
	exists           bool
	// seen is the text of the last frame, to tell edits; editAt is when
	// the last one came, and saving is a save waiting for typing to pause.
	seen   string
	editAt time.Time
	saving bool
	// carets keep where the caret was in each note opened this run;
	// caretPending puts it back in the next frame.
	carets       map[string]int
	caretPending int
	err          string

	// The list of notes (Cmd-P), a popover over the open note: what is
	// searched, the chosen row, and reveal to scroll it into view once,
	// after it changed, so that the wheel can move the list.
	listOpen bool
	query    string
	listIdx  int
	reveal   bool
	// editorH is the editor's height as last laid out with the list
	// closed: a note previewed from the list takes it, so that the panel
	// stays still. 0 before the editor was laid out.
	editorH float32
	// order is the order Cmd-[ and Cmd-] step through, taken at the first
	// step, so that stepping does not reorder it; an edit drops it.
	order []string

	in, searchIn ui.Handle
	// focusNote and focusSearch give the keyboard to the editor or the
	// search as they are next built: a note's editor is made anew for it.
	focusNote, focusSearch bool
}

// noteMode reports whether the notes tab is the current one: it comes after
// the modes.
func (p *Panel) noteMode() bool { return p.modeIdx == len(p.modes) }

// saveDelay is how long typing pauses before a note is saved.
const saveDelay = 500 * time.Millisecond

// setMode makes the mode at i current, with its own result, or notes at
// len(p.modes), saving the note being left.
func (p *Panel) setMode(i int) {
	was := p.noteMode()
	p.modeIdx = i
	if !p.noteMode() {
		p.showResult(p.modes[i].ID)
	}
	switch {
	case was && !p.noteMode():
		p.notes.listOpen = false
		p.leaveNote()
		p.sourceIn.Focus()
	case !was && p.noteMode():
		p.enterNotes()
	}
}

// enterNotes shows the note last edited, listing the notes again for what
// changed on disk meanwhile.
func (p *Panel) enterNotes() {
	p.notes.focusNote = true
	p.refreshNotes()
}

// setNotesDir moves the notes to dir's store; the next listing opens the
// latest of its notes.
func (p *Panel) setNotesDir(dir string) {
	if p.notes.store != nil && p.notes.store.Dir == dir {
		return
	}
	if p.notes.store != nil {
		p.leaveNote()
	}
	p.notes = notes{store: note.NewStore(dir, p.deps.Trash), carets: map[string]int{}, caretPending: -1}
	if p.noteMode() {
		p.refreshNotes()
	}
}

// refreshNotes lists the notes off the main thread, which a folder synced
// from elsewhere may hold up.
func (p *Panel) refreshNotes() {
	store := p.notes.store
	if store == nil {
		return
	}
	p.background(func() {
		list, err := store.List()
		p.post(func() {
			if p.notes.store != store {
				return
			}
			if err != nil {
				p.deps.Log.Warn("list notes", "err", err)
				p.notes.err = "读不到笔记目录"
			}
			p.listed(list)
		})
	})
}

// listed takes a listing: the open note is read again when its file
// changed elsewhere and it has no edits of its own.
func (p *Panel) listed(list []note.Note) {
	n := &p.notes
	n.all, n.loaded = list, true
	if n.cur == "" {
		if len(list) > 0 && n.text == "" {
			p.openNote(list[0].Name)
		} else {
			p.startNote(n.text)
		}
		return
	}
	for _, f := range list {
		if f.Name == n.cur && n.text == n.saved && f.Text != n.saved {
			n.text, n.saved, n.seen, n.exists = f.Text, f.Text, f.Text, true
		}
	}
}

// openNote shows a note of the list, with the caret where it was left.
func (p *Panel) openNote(name string) {
	n := &p.notes
	if name == n.cur {
		return
	}
	p.leaveNote()
	for _, f := range n.all {
		if f.Name == name {
			n.cur, n.text, n.saved, n.seen, n.exists = f.Name, f.Text, f.Text, f.Text, true
			n.caretPending, n.focusNote = n.carets[name], !n.listOpen
			p.deps.Log.Info("note open", "name", name)
			return
		}
	}
}

// newNote starts a blank note, unless the open one is blank already.
func (p *Panel) newNote() {
	n := &p.notes
	n.listOpen = false
	n.focusNote = true
	if n.cur != "" && strings.TrimSpace(n.text) == "" {
		return
	}
	p.leaveNote()
	p.startNote("")
}

// startNote opens a note that has no file yet, holding text.
func (p *Panel) startNote(text string) {
	n := &p.notes
	if n.store == nil {
		return
	}
	n.cur = n.store.NewName(func(name string) bool {
		for _, f := range n.all {
			if f.Name == name {
				return true
			}
		}
		return false
	})
	n.text, n.saved, n.seen, n.exists = text, "", text, false
	n.caretPending, n.focusNote = utf8.RuneCountInString(text), !n.listOpen
	n.order = nil
	if text != "" {
		p.saveNote()
	}
	p.deps.Log.Info("note new", "name", n.cur)
}

// leaveNote saves the open note as it is left, and removes it when it was
// emptied: there is nothing in it to get back.
func (p *Panel) leaveNote() {
	n := &p.notes
	if n.cur == "" {
		return
	}
	if strings.TrimSpace(n.text) != "" {
		p.saveNote()
		return
	}
	if n.exists {
		if err := n.store.Remove(n.cur); err != nil {
			p.deps.Log.Warn("remove empty note", "err", err)
			return
		}
		p.dropNote(n.cur)
		n.exists, n.saved = false, n.text
		p.deps.Log.Info("note removed empty", "name", n.cur)
	}
}

// saveNote writes the open note if it changed. A blank note is not
// written as it is typed; leaving it removes it.
func (p *Panel) saveNote() {
	n := &p.notes
	if n.cur == "" || n.text == n.saved || strings.TrimSpace(n.text) == "" {
		return
	}
	start := time.Now()
	mod, err := n.store.Write(n.cur, n.text)
	if err != nil {
		p.deps.Log.Warn("save note", "err", err)
		n.err = "笔记没有保存，稍后会重试"
		return
	}
	n.saved, n.exists, n.err = n.text, true, ""
	p.dropNote(n.cur)
	n.all = append([]note.Note{{Name: n.cur, Text: n.text, Modified: mod}}, n.all...)
	note.Sort(n.all)
	p.deps.Log.Debug("note saved", "name", n.cur, "took", time.Since(start))
}

// dropNote takes a note out of the list.
func (p *Panel) dropNote(name string) {
	all := p.notes.all[:0]
	for _, f := range p.notes.all {
		if f.Name != name {
			all = append(all, f)
		}
	}
	p.notes.all = all
}

// noteEdited saves the note once typing pauses, from the frame that sees
// an edit.
func (p *Panel) noteEdited() {
	n := &p.notes
	if n.text == n.seen {
		return
	}
	n.seen, n.editAt, n.order = n.text, time.Now(), nil
	if n.saving {
		return
	}
	n.saving = true
	p.after(saveDelay, p.saveWhenPaused)
}

func (p *Panel) saveWhenPaused() {
	n := &p.notes
	if wait := saveDelay - time.Since(n.editAt); wait > 0 {
		p.after(wait, p.saveWhenPaused)
		return
	}
	n.saving = false
	p.saveNote()
}

// deleteNote moves a note to the Trash; the open one gives way to the
// latest of the others, or a blank note.
func (p *Panel) deleteNote(name string) {
	n := &p.notes
	open := name == n.cur
	if open && !n.exists {
		n.text = ""
		return
	}
	if err := n.store.Delete(name); err != nil {
		p.deps.Log.Warn("delete note", "err", err)
		n.err = "没能移到废纸篓"
		return
	}
	p.dropNote(name)
	delete(n.carets, name)
	p.deps.Log.Info("note trashed", "name", name)
	if !open {
		return
	}
	n.cur, n.text, n.saved, n.seen, n.exists = "", "", "", "", false
	if len(n.all) > 0 {
		p.openNote(n.all[0].Name)
	} else {
		p.startNote("")
	}
}

// stepNote opens the note before (-1) or after (1) the open one, in the
// order of the last edits (Cmd-[ and Cmd-]).
func (p *Panel) stepNote(step int) {
	n := &p.notes
	p.leaveNote()
	if n.order == nil {
		for _, f := range n.all {
			n.order = append(n.order, f.Name)
		}
	}
	if len(n.order) == 0 {
		return
	}
	at := -1
	for i, name := range n.order {
		if name == n.cur {
			at = i
		}
	}
	// The notes go from the latest down: Cmd-[ goes back to older ones.
	to := at - step
	if at < 0 {
		to = 0
	}
	if to < 0 || to >= len(n.order) {
		return
	}
	order := n.order
	p.openNote(order[to])
	n.order = order
}

// openList shows the list of notes, searching from scratch.
func (p *Panel) openList() {
	n := &p.notes
	n.listOpen, n.query, n.listIdx, n.reveal = true, "", 0, true
	n.order = nil
	n.focusSearch = true
	p.refreshNotes()
}

// toggleList opens the list, or closes it back to the open note.
func (p *Panel) toggleList() {
	if p.notes.listOpen {
		p.closeList()
	} else {
		p.openList()
	}
}

// closeList goes back to the open note, as it was.
func (p *Panel) closeList() {
	n := &p.notes
	n.listOpen = false
	n.focusNote = true
	if n.cur != "" {
		n.caretPending = n.carets[n.cur]
	}
}

// listRows are the notes the search finds, the open one first, and whether
// its last row makes a note of what was searched. The open note leads, so
// that the row after it is the one before.
func (p *Panel) listRows() ([]note.Match, bool) {
	n := &p.notes
	all := make([]note.Note, 0, len(n.all)+1)
	if n.cur != "" && strings.TrimSpace(n.text) != "" {
		mod := time.Now()
		for _, f := range n.all {
			if f.Name == n.cur {
				mod = f.Modified
			}
		}
		all = append(all, note.Note{Name: n.cur, Text: n.text, Modified: mod})
	}
	for _, f := range n.all {
		if f.Name != n.cur {
			all = append(all, f)
		}
	}
	return note.Search(all, n.query), strings.TrimSpace(n.query) != ""
}

// listKey handles a key in the list's search field.
func (p *Panel) listKey(k ui.TextKey) bool {
	n := &p.notes
	if k.Text() != n.query {
		n.listIdx, n.reveal = 0, true
	}
	n.query = k.Text()
	rows, create := p.listRows()
	count := len(rows)
	if create {
		count++
	}
	switch {
	case k.Key == ui.KeyEscape && k.Mods == 0:
		p.closeList()
	case k.Key == ui.KeyTab && (k.Mods == 0 || k.Mods == ui.Shift):
		n.listOpen = false
		p.cycleMode(map[bool]int{true: -1, false: 1}[k.Mods == ui.Shift])
	case count == 0:
		return false
	case k.Key == ui.KeyDown && k.Mods == 0:
		n.listIdx, n.reveal = (n.listIdx+1)%count, true
	case k.Key == ui.KeyUp && k.Mods == 0:
		n.listIdx, n.reveal = (n.listIdx-1+count)%count, true
	case k.Key == ui.KeyEnter && k.Mods == 0:
		p.takeRow(rows, min(n.listIdx, count-1))
	case k.Key == ui.KeyBackspace && k.Mods == ui.Cmd:
		if i := min(n.listIdx, count-1); i < len(rows) {
			p.deleteNote(rows[i].Name)
		}
	default:
		return false
	}
	return true
}

// takeRow opens the note of row i, or, past the notes, makes one of the
// search.
func (p *Panel) takeRow(rows []note.Match, i int) {
	n := &p.notes
	query := strings.TrimSpace(n.query)
	p.closeList()
	if i < len(rows) {
		p.openNote(rows[i].Name)
		return
	}
	p.leaveNote()
	p.startNote(query)
}

// noteView is the open note, which stays as it is under the list.
func (p *Panel) noteView(c *ui.Context, pal *design.Palette) {
	n := &p.notes
	if m, ok := p.chosenNote(); ok && m.Name != n.cur {
		// The note chosen in the list shows in the editor's place, at the
		// editor's height, until Enter opens it or Esc goes back.
		text := m.Text
		pre := p.noteText(c, pal, ui.TextAreaBase(c.Key("note-preview-"+m.Name), &text).ReadOnly(true).Label("笔记预览"), text)
		if n.editorH > 0 && p.fixedHeight == 0 {
			pre.Height(n.editorH)
		} else {
			p.noteSize(pre)
		}
		return
	}
	in := p.noteText(c, pal, ui.TextAreaBase(c.Key("note-"+n.cur), &n.text).Bind(&n.in).
		Placeholder("随手记点什么…  ⌘N 新建  ⌘P 全部笔记").Label("笔记"), n.text)
	in.OnTextKey(func(k ui.TextKey) bool {
		n.text = k.Text()
		switch {
		case k.Key == ui.KeyTab && k.Mods == 0:
			p.cycleMode(1)
		case k.Key == ui.KeyTab && k.Mods == ui.Shift:
			p.cycleMode(-1)
		default:
			return false
		}
		return true
	})
	p.noteSize(in)
	// Bounds are the last frame's, empty in the frame after a preview, as
	// the editor is made anew: only a settled editor sets the height.
	if h := in.Bounds().H; h > 0 && !n.listOpen {
		n.editorH = h
	}
	if n.focusNote {
		in.Focus()
		n.focusNote = false
	}
	if n.caretPending >= 0 {
		in.SetTextSelection(n.caretPending, n.caretPending)
		n.caretPending = -1
	}
	if n.cur != "" {
		_, caret := in.TextAnchorCaret()
		n.carets[n.cur] = caret
	}
	if in.Clicked() && in.ClickModifiers()&ui.Cmd != 0 {
		_, caret := in.TextAnchorCaret()
		if url := note.URLAt(n.text, caret); url != "" {
			p.deps.Log.Info("note link opened")
			p.openURL(url)
		}
	}
	p.noteEdited()
}

// noteSize sizes a note's text area: it fills a panel the user resized, and
// follows the text otherwise, from 8 lines to what the screen has room for.
func (p *Panel) noteSize(in ui.Element) {
	if p.fixedHeight > 0 {
		in.Grow(1).Shrink(1).MinHeight(0)
		return
	}
	line := p.sz.Source * design.LineHeightSource
	maxLines := 12
	if p.place.maxHeight > 0 {
		maxLines = max(6, int((float32(p.place.maxHeight)-design.ControlBarHeight-2*design.SpaceSourceY)/line))
	}
	in.Lines(min(8, maxLines), maxLines)
}

// noteText styles a note's text area, the editor or a preview.
func (p *Panel) noteText(c *ui.Context, pal *design.Palette, in ui.Element, text string) ui.Element {
	return in.Shrink(0).FontSize(p.sz.Source).LineHeight(design.LineHeightSource).TextColor(pal.Bright).
		PaddingX(design.SpaceX).PaddingY(design.SpaceSourceY).
		TextRanges(noteRanges(text, pal)...)
}

// chosenNote is the note of the list's chosen row, while the list is open.
func (p *Panel) chosenNote() (note.Match, bool) {
	if !p.notes.listOpen {
		return note.Match{}, false
	}
	rows, create := p.listRows()
	if i := listSel(p.notes.listIdx, len(rows), create); i >= 0 && i < len(rows) {
		return rows[i], true
	}
	return note.Match{}, false
}

// listSel is the chosen row of n notes and, when create, the row that makes
// one after them, which is chosen when no note is found; -1 for no rows.
func listSel(idx, n int, create bool) int {
	count := n
	if create {
		count++
	}
	switch {
	case count == 0:
		return -1
	case create && n == 0:
		return count - 1
	}
	return min(idx, count-1)
}

// noteRanges colors a note's Markdown, which stays as typed.
func noteRanges(text string, pal *design.Palette) []ui.TextRange {
	spans := note.Highlight(text)
	ranges := make([]ui.TextRange, 0, len(spans))
	for _, s := range spans {
		r := ui.TextRange{Start: s.Start, End: s.End}
		switch s.Kind {
		case note.Heading:
			r.Color, r.Weight = pal.Bright, 700
		case note.Mark:
			r.Color = pal.Accent
		case note.Strong:
			r.Weight = 700
		case note.Code:
			r.Color = pal.Code
		case note.Quote:
			r.Color = pal.Quote
		case note.Link:
			r.Color = pal.Accent
		}
		ranges = append(ranges, r)
	}
	return ranges
}

// listWidth is the width of the list of notes.
const listWidth = 400

// listView is the list of notes over the button that opens it, as Raycast
// Notes shows its own: the search on top, the open note first, then the one
// before, and a row that makes a note of what was searched.
func (p *Panel) listView(c *ui.Context, pal *design.Palette, button ui.Element) {
	n := &p.notes
	pop := ui.PopoverBase(c, button, &n.listOpen, func(panel ui.Element) {
		// Above the bar, clear of its divider, in what the panel has left
		// over it with a margin at the top.
		const gap = 16
		room := max(120, p.panelH-design.ControlBarHeight-2*gap)
		panel.AttachTo(button, ui.AnchorTopRight, ui.AnchorBottomRight).Bottom(gap).
			Width(listWidth).MaxHeight(room).Background(pal.Surface).Border(1, pal.Border).Radius(design.RadiusPanel).Clip()
		panel.Children(func() { p.listContent(c, pal) })
	})
	if pop.Dismissed() {
		// A press outside goes on to what is under it, which takes the
		// keyboard if it wants it.
		n.reveal = false
	}
}

func (p *Panel) listContent(c *ui.Context, pal *design.Palette) {
	n := &p.notes
	rows, create := p.listRows()
	sel := listSel(n.listIdx, len(rows), create)
	ui.Row(c).Shrink(0).PaddingX(12).PaddingY(10).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, searchIcon).Size(14, 14).TextColor(pal.Faint)
		search := ui.TextInputBase(c.Key("note-search"), &n.query).Bind(&n.searchIn).Grow(1).
			Placeholder("搜索或新建").Label("搜索笔记").
			FontSize(p.sz.Body).TextColor(pal.Bright).
			OnTextKey(p.listKey)
		if n.focusSearch {
			search.Focus()
			n.focusSearch = false
		}
	})
	ui.Divider(c).Background(pal.Border)
	now := time.Now()
	ui.Scroll(c.Key("rows")).Grow(1).Shrink(1).MinHeight(0).Padding(4, 6, 6, 6).Gap(1).Children(func() {
		group := ""
		for i, m := range rows {
			// The rows go by when they were edited, under the day they
			// fall on, which their times then leave out.
			if g := dayGroup(m.Modified, now); g != group {
				group = g
				ui.Text(c, g).FontSize(p.sz.Caption).TextColor(pal.Faint).Padding(8, 10, 3, 10)
			}
			p.listRow(c, pal, i, m, i == sel, now)
		}
		if create {
			row := ui.Row(c.Key("create")).Gap(8).Padding(8, 10).Radius(design.RadiusBlock).AlignItems(ui.Center)
			p.rowState(row, pal, sel == len(rows))
			row.Children(func() {
				ui.Icon(c, plusIcon).Size(14, 14).TextColor(pal.Accent)
				ui.Text(c, strings.TrimSpace(n.query)).FontSize(p.sz.Label).FontWeight(600).TextColor(pal.Bright).Grow(1).MaxLines(1)
			})
			if row.Clicked() {
				p.takeRow(rows, len(rows))
			}
		}
	})
	n.reveal = false
	ui.Divider(c).Background(pal.Border)
	ui.Row(c).Shrink(0).PaddingX(12).PaddingY(7).Children(func() {
		ui.Spacer(c)
		p.kbd(c, pal, "⏎", "打开")
		if sel >= 0 && sel < len(rows) {
			p.kbd(c, pal, "⌘⌫", "删除")
		}
	})
}

// rowState colors a row of the list as chosen, with a bar at its start,
// or as hovered, and brings a newly chosen one into view.
func (p *Panel) rowState(row ui.Element, pal *design.Palette, chosen bool) {
	switch {
	case chosen:
		row.Background(pal.Accent.Alpha(0.14)).BorderStartWidth(2).BorderColor(pal.Accent)
		if p.notes.reveal {
			row.ScrollIntoView()
		}
	case row.Hovered():
		row.Background(pal.Raised.Alpha(0.6)).BorderStartWidth(2).BorderColor(pal.Accent.Alpha(0))
	default:
		row.BorderStartWidth(2).BorderColor(pal.Accent.Alpha(0))
	}
}

// dayGroup is the heading of the notes edited on t's day.
func dayGroup(t, now time.Time) string {
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	switch {
	case !t.Before(today):
		return "今天"
	case !t.Before(today.AddDate(0, 0, -1)):
		return "昨天"
	case !t.Before(today.AddDate(0, 0, -6)):
		return "最近 7 天"
	}
	return "更早"
}

// rowTime is when a note was edited, as its row under dayGroup shows it.
func rowTime(t, now time.Time) string {
	switch dayGroup(t, now) {
	case "今天":
		return ago(t, now)
	case "昨天":
		return t.Format("15:04")
	}
	return ago(t, now)
}

// listRow is a note of the list: its title and when it was edited, and a
// line of it below; the open note has a dot.
func (p *Panel) listRow(c *ui.Context, pal *design.Palette, i int, m note.Match, selected bool, now time.Time) {
	row := ui.Column(c.Key(m.Name)).Gap(3).Padding(8, 10).Radius(design.RadiusBlock)
	p.rowState(row, pal, selected)
	row.Children(func() {
		ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
			title := m.Note.Title()
			if title == "" {
				title = "空白笔记"
			}
			// A long title gives way to the dot and the time.
			marked(c, title, m.Title, pal).FontSize(p.sz.Label).FontWeight(600).TextColor(pal.Bright).
				Grow(1).Shrink(1).MinWidth(0).MaxLines(1)
			if m.Name == p.notes.cur {
				ui.Box(c).Size(6, 6).Shrink(0).Radius(3).Background(pal.Accent).Label("当前笔记")
			}
			ui.Text(c, rowTime(m.Modified, now)).FontSize(p.sz.Caption).TextColor(pal.Faint).Shrink(0)
		})
		switch line := note.Summary(m.Text); {
		case m.Line != "":
			marked(c, m.Line, m.Spans, pal).FontSize(p.sz.Caption).TextColor(pal.Muted).MinWidth(0).MaxLines(1)
		case line != "":
			ui.Text(c, line).FontSize(p.sz.Caption).TextColor(pal.Muted).MinWidth(0).MaxLines(1)
		}
	})
	if row.Clicked() {
		rows, _ := p.listRows()
		p.takeRow(rows, i)
	}
}

// marked is text with the runes the search found in it highlighted.
func marked(c *ui.Context, s string, spans [][2]int, pal *design.Palette) ui.Element {
	if len(spans) == 0 {
		return ui.Text(c, s)
	}
	rs := []rune(s)
	var parts []ui.Span
	at := 0
	for _, sp := range spans {
		parts = append(parts, ui.Span{Text: string(rs[at:sp[0]])},
			ui.Span{Text: string(rs[sp[0]:sp[1]]), Color: pal.Accent, Weight: 700})
		at = sp[1]
	}
	parts = append(parts, ui.Span{Text: string(rs[at:])})
	return ui.RichText(c, parts...)
}

// ago says how long ago t was, as the list shows when a note was edited.
func ago(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "刚刚"
	case d < time.Hour:
		return strconv.Itoa(int(d/time.Minute)) + " 分钟前"
	case t.YearDay() == now.YearDay() && t.Year() == now.Year():
		return t.Format("15:04")
	case now.Sub(t) < 48*time.Hour && t.YearDay() == now.Add(-24*time.Hour).YearDay():
		return "昨天"
	case t.Year() == now.Year():
		return t.Format("1月2日")
	}
	return t.Format("2006年1月2日")
}

var searchIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"><circle cx="7" cy="7" r="4.5"/><path d="M10.5 10.5l3 3"/></svg>`))

var plusIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"><path d="M8 3.5v9M3.5 8h9"/></svg>`))

var listIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round"><path d="M3 4.5h10M3 8h10M3 11.5h6"/></svg>`))

// noteBar is the right of the mode bar in notes mode: a save error, if any,
// and the buttons for a new note and for the list. The open note's title is not repeated here:
// it is the note's first line, in view above.
func (p *Panel) noteBar(c *ui.Context, pal *design.Palette) {
	n := &p.notes
	if n.err != "" {
		ui.Text(c, n.err).FontSize(p.sz.Caption).TextColor(pal.Danger).MarginEnd(10)
	}
	col := pal.Muted
	if n.listOpen {
		col = pal.Accent
	}
	p.iconButton(c, pal, "note-new", plusIcon, "新建笔记 ⌘N", pal.Muted, p.newNote).MarginEnd(2)
	list := p.iconButton(c, pal, "note-list", listIcon, "全部笔记 ⌘P", col, p.toggleList).MarginEnd(2)
	p.listView(c, pal, list)
}
