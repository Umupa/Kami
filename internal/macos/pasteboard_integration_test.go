//go:build integration

package macos

import (
	"bytes"
	"testing"
)

// The pasteboard is private: tests never touch the user's clipboard.
func TestPasteboardSnapshotRestore(t *testing.T) {
	p, err := NewPrivatePasteboard()
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	before := Snapshot{
		{Types: []string{"public.utf8-plain-text", "public.html"}, Data: [][]byte{[]byte("你好 hello"), []byte("<b>hi</b>")}},
		{Types: []string{"com.example.custom"}, Data: [][]byte{{0, 1, 2, 255}}},
	}
	count := p.ChangeCount()
	p.Restore(before, false)
	if p.ChangeCount() == count {
		t.Error("change count did not change")
	}
	if got := p.String(); got != "你好 hello" {
		t.Errorf("String() = %q", got)
	}

	snap := p.Snapshot()
	p.WriteString("copied")
	if p.String() != "copied" {
		t.Fatalf("WriteString did not replace the contents")
	}
	p.Restore(snap, true)
	after := p.Snapshot()
	if len(after) != 2 {
		t.Fatalf("restored %d items, want 2", len(after))
	}
	want := map[string][]byte{"public.utf8-plain-text": []byte("你好 hello"), "public.html": []byte("<b>hi</b>"), "com.example.custom": {0, 1, 2, 255}}
	transient := false
	for _, it := range after {
		for i, typ := range it.Types {
			if typ == TransientType {
				transient = true
				continue
			}
			if w, ok := want[typ]; ok && !bytes.Equal(it.Data[i], w) {
				t.Errorf("%s = %q, want %q", typ, it.Data[i], w)
			}
		}
	}
	if !transient {
		t.Error("restored contents are not marked transient")
	}
}

func TestReadOnlyQueries(t *testing.T) {
	t.Logf("AX trusted: %v, secure input: %v, frontmost is self: %v", AXTrusted(), SecureInput(), FrontmostIsSelf())
	if AXTrusted() {
		f, err := FocusedElement(150e6)
		t.Logf("focused role %q, has selection %v, err %v", f.Role, f.HasSelection, err)
	}
}
