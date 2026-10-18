package macos

import (
	"os"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	tisCopyCurrentKeyboardLayoutInputSource func() uintptr
	tisGetInputSourceProperty               func(source, key uintptr) uintptr
	tisUnicodeKeyLayoutData                 uintptr
	lmGetKbdType                            func() uint8
	ucKeyTranslate                          func(layout unsafe.Pointer, keyCode, action uint16, modifiers, kbdType, options uint32, deadKeyState *uint32, maxLen int, actualLen *int, chars *uint16) int32
	cfDataGetBytePtr                        func(data uintptr) unsafe.Pointer
	isSecureEventInputEnabled               func() bool

	cgEventCreateKeyboardEvent func(source uintptr, key uint16, down bool) uintptr
	cgEventSetFlags            func(event uintptr, flags uint64)
	cgEventPost                func(tap uint32, event uintptr)
)

func loadKeyboard() {
	purego.RegisterLibFunc(&tisCopyCurrentKeyboardLayoutInputSource, libCarbon, "TISCopyCurrentKeyboardLayoutInputSource")
	purego.RegisterLibFunc(&tisGetInputSourceProperty, libCarbon, "TISGetInputSourceProperty")
	purego.RegisterLibFunc(&lmGetKbdType, libCarbon, "LMGetKbdType")
	purego.RegisterLibFunc(&ucKeyTranslate, libCarbon, "UCKeyTranslate")
	purego.RegisterLibFunc(&isSecureEventInputEnabled, libCarbon, "IsSecureEventInputEnabled")
	purego.RegisterLibFunc(&cfDataGetBytePtr, libCF, "CFDataGetBytePtr")
	if p, err := purego.Dlsym(libCarbon, "kTISPropertyUnicodeKeyLayoutData"); err == nil {
		tisUnicodeKeyLayoutData = **(**uintptr)(unsafe.Pointer(&p))
	}
	purego.RegisterLibFunc(&cgEventCreateKeyboardEvent, libCG, "CGEventCreateKeyboardEvent")
	purego.RegisterLibFunc(&cgEventSetFlags, libCG, "CGEventSetFlags")
	purego.RegisterLibFunc(&cgEventPost, libCG, "CGEventPost")
}

// ansiC is the key code of C on a US keyboard, used when the layout has none.
const ansiC = 8

// KeyCodeFor returns the key code that types r on the current keyboard
// layout, so that a synthetic Cmd-C works on Dvorak and AZERTY too. The
// input source APIs must be called on the main thread.
func KeyCodeFor(r rune) uint16 {
	if openFrameworks() != nil || tisUnicodeKeyLayoutData == 0 {
		return ansiC
	}
	src := tisCopyCurrentKeyboardLayoutInputSource()
	if src == 0 {
		return ansiC
	}
	defer cfRelease(src)
	data := tisGetInputSourceProperty(src, tisUnicodeKeyLayoutData)
	if data == 0 {
		return ansiC
	}
	layout := cfDataGetBytePtr(data)
	kbd := uint32(lmGetKbdType())
	const actionDisplay, noDeadKeys = 3, 1
	for code := uint16(0); code < 128; code++ {
		var dead uint32
		var n int
		var chars [4]uint16
		if ucKeyTranslate(layout, code, actionDisplay, 0, kbd, noDeadKeys, &dead, len(chars), &n, &chars[0]) == 0 && n == 1 && rune(chars[0]) == r {
			return code
		}
	}
	return ansiC
}

// SecureInput reports whether an app has turned on secure input, as password
// fields do: synthetic copies must not be sent then.
func SecureInput() bool {
	return openFrameworks() == nil && isSecureEventInputEnabled()
}

const (
	hidEventTap     = 0
	flagMaskCommand = 0x00100000
)

// PressCommand posts key down and up with Command alone, whatever modifiers
// the user is still holding from the shortcut.
func PressCommand(keyCode uint16) {
	if openFrameworks() != nil {
		return
	}
	for _, down := range []bool{true, false} {
		ev := cgEventCreateKeyboardEvent(0, keyCode, down)
		if ev == 0 {
			return
		}
		cgEventSetFlags(ev, flagMaskCommand)
		cgEventPost(hidEventTap, ev)
		cfRelease(ev)
	}
}

// FrontmostIsSelf reports whether this process is the frontmost app.
func FrontmostIsSelf() bool {
	if openFrameworks() != nil {
		return false
	}
	var pid int32
	withPool(func() {
		app := send(send(class("NSWorkspace"), "sharedWorkspace"), "frontmostApplication")
		if app != 0 {
			pid = objc.Send[int32](app, sel("processIdentifier"))
		}
	})
	return int(pid) == os.Getpid()
}
