package hotkey

import (
	"time"

	"github.com/Umupa/Gloss/internal/config"
)

// DoubleTapWindow is how quickly a double tap is made, from the first press
// to the second release.
const DoubleTapWindow = 300 * time.Millisecond

// DoubleTap recognizes one modifier pressed and released twice, alone: no
// other key or modifier in between, all within Window.
type DoubleTap struct {
	Mod    config.Modifiers
	Window time.Duration

	step  int // 1 first press, 2 first release, 3 second press
	start time.Time
	prev  config.Modifiers
}

// Feed takes an event: the modifiers held after it, and whether it pressed a
// key that is not a modifier. It reports whether the event completes a
// double tap.
func (d *DoubleTap) Feed(at time.Time, mods config.Modifiers, key bool) bool {
	if key {
		d.step = 0
		return false
	}
	if mods == d.prev {
		// A modifier the detector ignores, as Fn or Caps Lock, changed.
		return false
	}
	pressed := d.prev == 0 && mods == d.Mod // the modifier alone, from nothing
	d.prev = mods
	if d.step > 0 && at.Sub(d.start) > d.Window {
		d.step = 0
	}
	switch {
	case pressed && d.step == 2:
		d.step = 3
	case pressed:
		d.step, d.start = 1, at
	case mods == 0 && d.step == 1:
		d.step = 2
	case mods == 0 && d.step == 3:
		d.step = 0
		return true
	default:
		d.step = 0
	}
	return false
}

// Flag masks of CGEventFlags for the modifiers a shortcut can use.
const (
	flagShift   = 0x00020000
	flagControl = 0x00040000
	flagOption  = 0x00080000
	flagCommand = 0x00100000
)

// ModifiersOf returns the modifiers in CGEventFlags, leaving out Caps Lock,
// Fn and the rest.
func ModifiersOf(flags uint64) config.Modifiers {
	var m config.Modifiers
	if flags&flagControl != 0 {
		m |= config.ModControl
	}
	if flags&flagOption != 0 {
		m |= config.ModOption
	}
	if flags&flagShift != 0 {
		m |= config.ModShift
	}
	if flags&flagCommand != 0 {
		m |= config.ModCommand
	}
	return m
}
