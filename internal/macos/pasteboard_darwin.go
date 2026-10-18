package macos

import (
	"github.com/ebitengine/purego/objc"
)

// TransientType marks pasteboard contents that clipboard managers should not
// record (nspasteboard.org).
const TransientType = "org.nspasteboard.TransientType"

const plainTextType = "public.utf8-plain-text"

// Pasteboard is an NSPasteboard. Its methods are safe from any goroutine.
type Pasteboard struct {
	id      objc.ID
	private bool
}

// GeneralPasteboard returns the system clipboard.
func GeneralPasteboard() (*Pasteboard, error) {
	if err := openFrameworks(); err != nil {
		return nil, err
	}
	var p objc.ID
	withPool(func() { p = send(send(class("NSPasteboard"), "generalPasteboard"), "retain") })
	return &Pasteboard{id: p}, nil
}

// NewPrivatePasteboard returns a pasteboard of a unique name, which no other
// app sees, for tests: they must never touch the user's clipboard. Close it.
func NewPrivatePasteboard() (*Pasteboard, error) {
	if err := openFrameworks(); err != nil {
		return nil, err
	}
	var p objc.ID
	withPool(func() { p = send(send(class("NSPasteboard"), "pasteboardWithUniqueName"), "retain") })
	return &Pasteboard{id: p, private: true}, nil
}

// Close releases the pasteboard, and removes a private one from the system.
func (p *Pasteboard) Close() {
	withPool(func() {
		if p.private {
			send(p.id, "releaseGlobally")
		}
		send(p.id, "release")
	})
}

// ChangeCount changes whenever the contents change.
func (p *Pasteboard) ChangeCount() int {
	var n int
	withPool(func() { n = objc.Send[int](p.id, sel("changeCount")) })
	return n
}

// String returns the plain text on the pasteboard, "" when there is none.
func (p *Pasteboard) String() string {
	var s string
	withPool(func() { s = goString(send(p.id, "stringForType:", nsString(plainTextType))) })
	return s
}

// Item is one pasteboard item: its data in every type it offers.
type Item struct {
	Types []string
	Data  [][]byte
}

// Snapshot is the whole contents of a pasteboard.
type Snapshot []Item

// Snapshot copies every item in every type, to put back with Restore.
func (p *Pasteboard) Snapshot() Snapshot {
	var s Snapshot
	withPool(func() {
		items := send(p.id, "pasteboardItems")
		n := objc.Send[int](items, sel("count"))
		for i := range n {
			item := send(items, "objectAtIndex:", i)
			types := send(item, "types")
			var it Item
			for j := range objc.Send[int](types, sel("count")) {
				t := send(types, "objectAtIndex:", j)
				data := send(item, "dataForType:", t)
				if data == 0 {
					continue
				}
				it.Types = append(it.Types, goString(t))
				it.Data = append(it.Data, goBytes(data))
			}
			s = append(s, it)
		}
	})
	return s
}

// Restore replaces the contents with s. transient marks them as not worth
// recording, for contents put back after a synthetic copy. It returns the
// change count after the write.
func (p *Pasteboard) Restore(s Snapshot, transient bool) int {
	var n int
	withPool(func() {
		send(p.id, "clearContents")
		objs := send(class("NSMutableArray"), "array")
		for i, it := range s {
			item := send(send(send(class("NSPasteboardItem"), "alloc"), "init"), "autorelease")
			for j, t := range it.Types {
				send(item, "setData:forType:", nsData(it.Data[j]), nsString(t))
			}
			if transient && i == 0 {
				send(item, "setData:forType:", nsData(nil), nsString(TransientType))
			}
			send(objs, "addObject:", item)
		}
		if objc.Send[int](objs, sel("count")) > 0 {
			send(p.id, "writeObjects:", objs)
		}
		n = objc.Send[int](p.id, sel("changeCount"))
	})
	return n
}

// WriteString replaces the contents with plain text, for tests.
func (p *Pasteboard) WriteString(s string) {
	p.Restore(Snapshot{{Types: []string{plainTextType}, Data: [][]byte{[]byte(s)}}}, false)
}
