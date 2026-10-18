package hotkey

import (
	"testing"
	"time"

	"github.com/Umupa/cida-next/internal/config"
)

// step is one event: ms after the start, the modifiers held after it, and
// whether a non-modifier key went down.
type step struct {
	ms   int
	mods config.Modifiers
	key  bool
}

const opt, ctl = config.ModOption, config.ModControl

func TestDoubleTap(t *testing.T) {
	tests := []struct {
		name  string
		steps []step
		want  []int // indexes of the steps that complete a double tap
	}{
		{"tap tap", []step{{0, opt, false}, {60, 0, false}, {150, opt, false}, {220, 0, false}}, []int{3}},
		{"too slow", []step{{0, opt, false}, {60, 0, false}, {250, opt, false}, {320, 0, false}}, nil},
		{"key between", []step{{0, opt, false}, {60, 0, false}, {100, 0, true}, {150, opt, false}, {220, 0, false}}, nil},
		{"key while held", []step{{0, opt, false}, {30, opt, true}, {60, 0, false}, {150, opt, false}, {220, 0, false}}, nil},
		{"other modifier joins", []step{{0, opt, false}, {20, opt | ctl, false}, {40, opt, false}, {60, 0, false}, {150, opt, false}, {220, 0, false}}, nil},
		{"other modifier", []step{{0, ctl, false}, {60, 0, false}, {150, ctl, false}, {220, 0, false}}, nil},
		{"three taps fire once", []step{{0, opt, false}, {50, 0, false}, {100, opt, false}, {150, 0, false}, {200, opt, false}, {250, 0, false}}, []int{3}},
		{"two double taps", []step{{0, opt, false}, {50, 0, false}, {100, opt, false}, {150, 0, false}, {600, opt, false}, {650, 0, false}, {700, opt, false}, {750, 0, false}}, []int{3, 7}},
		{"slow first tap restarts", []step{{0, opt, false}, {400, 0, false}, {450, opt, false}, {500, 0, false}, {550, opt, false}, {600, 0, false}}, []int{5}},
		{"ignored modifier changes", []step{{0, opt, false}, {20, opt, false}, {60, 0, false}, {80, 0, false}, {150, opt, false}, {220, 0, false}}, []int{5}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := &DoubleTap{Mod: opt, Window: DoubleTapWindow}
			start := time.Unix(0, 0)
			var got []int
			for i, s := range tt.steps {
				if d.Feed(start.Add(time.Duration(s.ms)*time.Millisecond), s.mods, s.key) {
					got = append(got, i)
				}
			}
			if len(got) != len(tt.want) {
				t.Fatalf("fired at %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("fired at %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestModifiersOf(t *testing.T) {
	const capsLock, fn = 0x00010000, 0x00800000
	if got := ModifiersOf(flagOption | capsLock | fn); got != config.ModOption {
		t.Errorf("option with caps lock and fn = %b", got)
	}
	if got := ModifiersOf(flagControl | flagShift | flagCommand); got != config.ModControl|config.ModShift|config.ModCommand {
		t.Errorf("control shift command = %b", got)
	}
}
