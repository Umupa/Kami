// Package macos binds the system frameworks MyGo does not cover, through
// purego. Other packages use the Go interfaces it exports.
package macos

import (
	"errors"
	"fmt"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

// KeyEvent is a key event seen by WatchKeys.
type KeyEvent struct {
	// Flags are the CGEventFlags after the event: the modifiers held.
	Flags uint64
	// KeyDown reports a key that is not a modifier going down; otherwise
	// the event is a modifier changing.
	KeyDown bool
}

// ErrNoListenAccess means Input Monitoring has not been granted.
var ErrNoListenAccess = errors.New("macos: Input Monitoring is not granted")

const (
	tapSession            = 1 // kCGSessionEventTap
	tapHeadInsert         = 0 // kCGHeadInsertEventTap
	tapListenOnly         = 1 // kCGEventTapOptionListenOnly
	eventKeyDown          = 10
	eventFlagsChanged     = 12
	eventTapDisabledByTO  = 0xFFFFFFFE
	eventTapDisabledByUsr = 0xFFFFFFFF
)

var (
	loadOnce sync.Once
	loadErr  error

	cgEventTapCreate              func(tap, place, options uint32, mask uint64, callback, info uintptr) uintptr
	cgEventTapEnable              func(port uintptr, enable bool)
	cgEventGetFlags               func(event uintptr) uint64
	cgPreflightListenEventAccess  func() bool
	cgRequestListenEventAccess    func() bool
	cfMachPortCreateRunLoopSource func(alloc, port uintptr, order int) uintptr
	cfRunLoopGetMain              func() uintptr
	cfRunLoopAddSource            func(loop, source, mode uintptr)
	cfRunLoopCommonModes          uintptr
)

func load() error {
	loadOnce.Do(func() {
		cg, err := purego.Dlopen("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			loadErr = err
			return
		}
		cf, err := purego.Dlopen("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation", purego.RTLD_GLOBAL|purego.RTLD_NOW)
		if err != nil {
			loadErr = err
			return
		}
		purego.RegisterLibFunc(&cgEventTapCreate, cg, "CGEventTapCreate")
		purego.RegisterLibFunc(&cgEventTapEnable, cg, "CGEventTapEnable")
		purego.RegisterLibFunc(&cgEventGetFlags, cg, "CGEventGetFlags")
		purego.RegisterLibFunc(&cgPreflightListenEventAccess, cg, "CGPreflightListenEventAccess")
		purego.RegisterLibFunc(&cgRequestListenEventAccess, cg, "CGRequestListenEventAccess")
		purego.RegisterLibFunc(&cfMachPortCreateRunLoopSource, cf, "CFMachPortCreateRunLoopSource")
		purego.RegisterLibFunc(&cfRunLoopGetMain, cf, "CFRunLoopGetMain")
		purego.RegisterLibFunc(&cfRunLoopAddSource, cf, "CFRunLoopAddSource")
		p, err := purego.Dlsym(cf, "kCFRunLoopCommonModes")
		if err != nil {
			loadErr = err
			return
		}
		cfRunLoopCommonModes = **(**uintptr)(unsafe.Pointer(&p))
	})
	return loadErr
}

// ListenAccess reports whether Input Monitoring is granted.
func ListenAccess() bool {
	return load() == nil && cgPreflightListenEventAccess()
}

// RequestListenAccess asks for Input Monitoring: the first time, macOS shows
// its prompt; after that it only lists the app in System Settings.
func RequestListenAccess() bool {
	return load() == nil && cgRequestListenEventAccess()
}

var (
	tapMu    sync.Mutex
	tapPort  uintptr
	tapFn    func(KeyEvent)
	callback = purego.NewCallback(onTapEvent)
)

// WatchKeys calls fn, on the main thread, for every key press and modifier
// change in the user's session. The tap only listens: it never delays, drops
// or changes an event. Call it on the main thread, once; fn must return
// quickly, or macOS turns the tap off (it is turned back on at once).
func WatchKeys(fn func(KeyEvent)) error {
	if err := load(); err != nil {
		return err
	}
	tapMu.Lock()
	defer tapMu.Unlock()
	if tapPort != 0 {
		return errors.New("macos: keys are already watched")
	}
	mask := uint64(1)<<eventKeyDown | uint64(1)<<eventFlagsChanged
	port := cgEventTapCreate(tapSession, tapHeadInsert, tapListenOnly, mask, callback, 0)
	if port == 0 {
		if !cgPreflightListenEventAccess() {
			return ErrNoListenAccess
		}
		return fmt.Errorf("macos: CGEventTapCreate failed")
	}
	source := cfMachPortCreateRunLoopSource(0, port, 0)
	if source == 0 {
		return fmt.Errorf("macos: CFMachPortCreateRunLoopSource failed")
	}
	cfRunLoopAddSource(cfRunLoopGetMain(), source, cfRunLoopCommonModes)
	cgEventTapEnable(port, true)
	tapPort, tapFn = port, fn
	return nil
}

func onTapEvent(proxy uintptr, typ uint32, event uintptr, info uintptr) uintptr {
	tapMu.Lock()
	port, fn := tapPort, tapFn
	tapMu.Unlock()
	switch typ {
	case eventTapDisabledByTO, eventTapDisabledByUsr:
		if port != 0 {
			cgEventTapEnable(port, true)
		}
	case eventKeyDown, eventFlagsChanged:
		if fn != nil {
			fn(KeyEvent{Flags: cgEventGetFlags(event), KeyDown: typ == eventKeyDown})
		}
	}
	return event
}
