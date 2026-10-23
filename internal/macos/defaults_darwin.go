package macos

import "github.com/ebitengine/purego/objc"

// DefaultBool reads a boolean from the app's user defaults; false when it is
// absent or the defaults cannot be read.
func DefaultBool(key string) bool {
	if openFrameworks() != nil {
		return false
	}
	var v bool
	withPool(func() {
		v = objc.Send[bool](send(class("NSUserDefaults"), "standardUserDefaults"), sel("boolForKey:"), nsString(key))
	})
	return v
}

// SetDefaultBool writes a boolean to the app's user defaults; false removes
// the key, which reads as false.
func SetDefaultBool(key string, v bool) {
	if openFrameworks() != nil {
		return
	}
	withPool(func() {
		d := send(class("NSUserDefaults"), "standardUserDefaults")
		if v {
			send(d, "setBool:forKey:", true, nsString(key))
		} else {
			send(d, "removeObjectForKey:", nsString(key))
		}
	})
}
