package macos

import (
	"errors"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var (
	axIsProcessTrusted            func() bool
	axIsProcessTrustedWithOptions func(options uintptr) bool
	axCreateSystemWide            func() uintptr
	axCopyAttributeValue          func(element, attribute uintptr, value *uintptr) int32
	axSetMessagingTimeout         func(element uintptr, seconds float32) int32
	axValueGetValue               func(value uintptr, typ int32, out unsafe.Pointer) bool
)

func loadAX() {
	purego.RegisterLibFunc(&axIsProcessTrusted, libAS, "AXIsProcessTrusted")
	purego.RegisterLibFunc(&axIsProcessTrustedWithOptions, libAS, "AXIsProcessTrustedWithOptions")
	purego.RegisterLibFunc(&axCreateSystemWide, libAS, "AXUIElementCreateSystemWide")
	purego.RegisterLibFunc(&axCopyAttributeValue, libAS, "AXUIElementCopyAttributeValue")
	purego.RegisterLibFunc(&axSetMessagingTimeout, libAS, "AXUIElementSetMessagingTimeout")
	purego.RegisterLibFunc(&axValueGetValue, libAS, "AXValueGetValue")
}

// ErrNoAccessibility means Accessibility has not been granted.
var ErrNoAccessibility = errors.New("macos: Accessibility is not granted")

// AXTrusted reports whether Accessibility is granted.
func AXTrusted() bool {
	return openFrameworks() == nil && axIsProcessTrusted()
}

// RequestAXTrust shows the system prompt that leads to the Accessibility
// settings, and reports whether access is granted already.
func RequestAXTrust() bool {
	if openFrameworks() != nil {
		return false
	}
	var trusted bool
	withPool(func() {
		key := nsString("AXTrustedCheckOptionPrompt")
		opts := send(class("NSDictionary"), "dictionaryWithObject:forKey:", send(class("NSNumber"), "numberWithBool:", true), key)
		trusted = axIsProcessTrustedWithOptions(uintptr(opts))
	})
	return trusted
}

// Focused is what Accessibility says about the focused element of the
// frontmost app.
type Focused struct {
	Role, Subrole string
	// Selected is its selected text; HasSelection reports that the element
	// answered for it, even with nothing selected.
	Selected     string
	HasSelection bool
	// SelectedLength is the length of its selected range, -1 when unknown.
	SelectedLength int
	// Value is its text, when it is short enough to compare with: the copy
	// fallback uses it to tell a copied line from a selection.
	Value string
}

// Secure reports a password field.
func (f Focused) Secure() bool {
	return f.Role == "AXSecureTextField" || f.Subrole == "AXSecureTextField"
}

const (
	axErrorSuccess   = 0
	axValueCFRange   = 4
	maxComparedValue = 1 << 20
)

// FocusedElement reads the focused element of the frontmost app, every call
// to the app bounded by timeout. It fails with ErrNoAccessibility without
// access, and with an error when there is no focused element.
func FocusedElement(timeout time.Duration) (Focused, error) {
	if err := openFrameworks(); err != nil {
		return Focused{}, err
	}
	if !axIsProcessTrusted() {
		return Focused{}, ErrNoAccessibility
	}
	var f Focused
	var err error
	withPool(func() {
		sys := axCreateSystemWide()
		defer cfRelease(sys)
		axSetMessagingTimeout(sys, float32(timeout.Seconds()))
		elem, ok := copyAttr(sys, "AXFocusedUIElement")
		if !ok {
			err = errors.New("macos: no focused element")
			return
		}
		defer cfRelease(elem)
		axSetMessagingTimeout(elem, float32(timeout.Seconds()))
		f.Role = stringAttr(elem, "AXRole")
		f.Subrole = stringAttr(elem, "AXSubrole")
		if f.Secure() {
			return
		}
		if v, ok := copyAttr(elem, "AXSelectedText"); ok {
			f.HasSelection = true
			if isString(objc.ID(v)) {
				f.Selected = goString(objc.ID(v))
			}
			cfRelease(v)
		}
		f.SelectedLength = -1
		if v, ok := copyAttr(elem, "AXSelectedTextRange"); ok {
			var r struct{ Location, Length int }
			if axValueGetValue(v, axValueCFRange, unsafe.Pointer(&r)) {
				f.SelectedLength = r.Length
			}
			cfRelease(v)
		}
		if v, ok := copyAttr(elem, "AXValue"); ok {
			if isString(objc.ID(v)) && objc.Send[int](objc.ID(v), sel("length")) <= maxComparedValue {
				f.Value = goString(objc.ID(v))
			}
			cfRelease(v)
		}
	})
	return f, err
}

// copyAttr copies an attribute's value, retained: release it.
func copyAttr(elem uintptr, name string) (uintptr, bool) {
	var v uintptr
	if axCopyAttributeValue(elem, uintptr(nsString(name)), &v) != axErrorSuccess || v == 0 {
		return 0, false
	}
	return v, true
}

func stringAttr(elem uintptr, name string) string {
	v, ok := copyAttr(elem, name)
	if !ok {
		return ""
	}
	defer cfRelease(v)
	if !isString(objc.ID(v)) {
		return ""
	}
	return goString(objc.ID(v))
}
