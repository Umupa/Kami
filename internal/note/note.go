// Package note keeps the notes: scratch text the user writes in the panel,
// one Markdown file each in a directory of the user's choice. It knows
// nothing of the panel, so that it runs and is tested without a window.
package note

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Ext is the extension of a note's file.
const Ext = ".md"

// Note is one note as it is on disk.
type Note struct {
	// Name is the file's name, which never changes: it is when the note
	// was made, so that its title can change without a rename.
	Name     string
	Text     string
	Modified time.Time
}

// Title is the note's first line without its leading Markdown marks.
func (n Note) Title() string { return Title(n.Text) }

// Store reads and writes the notes of a directory. Its methods are safe
// from one goroutine at a time.
type Store struct {
	Dir string
	// Trash moves a file to the Trash; nil removes it for good.
	Trash func(path string) error
	// now is the clock that names new notes.
	now func() time.Time
}

// NewStore returns the store of dir, which is made on the first write.
func NewStore(dir string, trash func(string) error) *Store {
	return &Store{Dir: dir, Trash: trash, now: time.Now}
}

// List reads every note, the most recently edited first.
func (s *Store) List() ([]Note, error) {
	entries, err := os.ReadDir(s.Dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var notes []Note
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), Ext) || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		n, err := s.Read(e.Name())
		if err != nil {
			continue
		}
		notes = append(notes, n)
	}
	Sort(notes)
	return notes, nil
}

// Sort orders notes the most recently edited first, newer names first
// among those edited at once.
func Sort(notes []Note) {
	slices.SortStableFunc(notes, func(a, b Note) int {
		if c := b.Modified.Compare(a.Modified); c != 0 {
			return c
		}
		return strings.Compare(b.Name, a.Name)
	})
}

// Read reads one note.
func (s *Store) Read(name string) (Note, error) {
	path := filepath.Join(s.Dir, name)
	info, err := os.Stat(path)
	if err != nil {
		return Note{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Note{}, err
	}
	return Note{Name: name, Text: string(data), Modified: info.ModTime()}, nil
}

// Write saves a note's text through a temporary file, so that a reader
// never sees half of it, and returns its modification time.
func (s *Store) Write(name, text string) (time.Time, error) {
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return time.Time{}, err
	}
	tmp, err := os.CreateTemp(s.Dir, ".tmp-*")
	if err != nil {
		return time.Time{}, err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(text); err != nil {
		tmp.Close()
		return time.Time{}, err
	}
	if err := tmp.Close(); err != nil {
		return time.Time{}, err
	}
	path := filepath.Join(s.Dir, name)
	if err := os.Rename(tmp.Name(), path); err != nil {
		return time.Time{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}

// NewName names a note made now, unlike any name in taken.
func (s *Store) NewName(taken func(string) bool) string {
	base := s.now().Format("2006-01-02-150405")
	name := base + Ext
	for i := 2; taken(name) || s.exists(name); i++ {
		name = fmt.Sprintf("%s-%d%s", base, i, Ext)
	}
	return name
}

func (s *Store) exists(name string) bool {
	_, err := os.Stat(filepath.Join(s.Dir, name))
	return err == nil
}

// Delete moves a note to the Trash.
func (s *Store) Delete(name string) error {
	path := filepath.Join(s.Dir, name)
	if s.Trash == nil {
		return os.Remove(path)
	}
	return s.Trash(path)
}

// Remove deletes the file of an emptied note for good: there is nothing in
// it to get back. A note never written is no error.
func (s *Store) Remove(name string) error {
	err := os.Remove(filepath.Join(s.Dir, name))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Title is text's first line that is not blank, without the Markdown marks
// that start it: a heading's #, a list's bullet or a quote's >.
func Title(text string) string {
	for line := range strings.Lines(text) {
		line = strings.TrimSpace(line)
		line = strings.TrimLeft(line, "#>")
		line = strings.TrimSpace(line)
		for _, mark := range []string{"- [ ] ", "- [x] ", "- [X] ", "- ", "* ", "+ "} {
			if strings.HasPrefix(line, mark) {
				line = strings.TrimSpace(line[len(mark):])
				break
			}
		}
		if line != "" {
			return line
		}
	}
	return ""
}

// Match is a note that has what was searched for.
type Match struct {
	Note
	// Title are the runes of the title that match, as [start, end) pairs.
	Title [][2]int
	// Line is the line of the text that matches, when the title does not,
	// and Spans the runes of it that match.
	Line  string
	Spans [][2]int
}

// Search returns the notes whose title or text has query, ignoring case, in
// the order given; all of them when query is blank.
func Search(notes []Note, query string) []Match {
	q := strings.ToLower(strings.TrimSpace(query))
	var out []Match
	for _, n := range notes {
		title := n.Title()
		if q == "" {
			out = append(out, Match{Note: n})
			continue
		}
		if spans := find(title, q); len(spans) > 0 {
			out = append(out, Match{Note: n, Title: spans})
			continue
		}
		for line := range strings.Lines(n.Text) {
			line = strings.TrimSpace(line)
			if spans := find(line, q); len(spans) > 0 {
				out = append(out, Match{Note: n, Line: line, Spans: spans})
				break
			}
		}
	}
	return out
}

// find returns where q, lower case, is in s, in runes.
func find(s, q string) [][2]int {
	rs := []rune(strings.ToLower(s))
	qs := []rune(q)
	if len(rs) != len([]rune(s)) {
		// A rune whose lower case is longer: compare rune by rune.
		rs = []rune(s)
		for i, r := range rs {
			rs[i] = []rune(strings.ToLower(string(r)))[0]
		}
	}
	var spans [][2]int
	for i := 0; i+len(qs) <= len(rs); {
		if slices.Equal(rs[i:i+len(qs)], qs) {
			spans = append(spans, [2]int{i, i + len(qs)})
			i += len(qs)
			continue
		}
		i++
	}
	return spans
}
