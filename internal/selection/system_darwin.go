package selection

import (
	"time"

	"github.com/Umupa/cida-next/internal/macos"
)

// macSystem is System on the running Mac.
type macSystem struct {
	*macos.Pasteboard
	keyCode uint16
}

// NewSystem returns the running Mac's System over the general pasteboard.
// keyCode is the key that types C on the current layout (macos.KeyCodeFor,
// which runs on the main thread).
func NewSystem(pb *macos.Pasteboard, keyCode uint16) System {
	return macSystem{Pasteboard: pb, keyCode: keyCode}
}

func (macSystem) FrontmostIsSelf() bool { return macos.FrontmostIsSelf() }
func (macSystem) SecureInput() bool     { return macos.SecureInput() }

func (macSystem) Focused(timeout time.Duration) (macos.Focused, error) {
	return macos.FocusedElement(timeout)
}

func (s macSystem) PressCopy() { macos.PressCommand(s.keyCode) }
