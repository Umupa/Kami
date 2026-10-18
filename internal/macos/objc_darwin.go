package macos

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Frameworks, opened once.
var (
	frameworksOnce sync.Once
	frameworksErr  error
	libAppKit      uintptr
	libAS          uintptr // ApplicationServices: Accessibility
	libCarbon      uintptr // HIToolbox: keyboard layouts, secure input
	libCG          uintptr
	libCF          uintptr

	autoreleasePoolPush func() uintptr
	autoreleasePoolPop  func(pool uintptr)
	cfRelease           func(ref uintptr)
)

func openFrameworks() error {
	frameworksOnce.Do(func() {
		open := func(path string) uintptr {
			if frameworksErr != nil {
				return 0
			}
			h, err := purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW)
			if err != nil {
				frameworksErr = err
			}
			return h
		}
		libAppKit = open("/System/Library/Frameworks/AppKit.framework/AppKit")
		libAS = open("/System/Library/Frameworks/ApplicationServices.framework/ApplicationServices")
		libCarbon = open("/System/Library/Frameworks/Carbon.framework/Carbon")
		libCG = open("/System/Library/Frameworks/CoreGraphics.framework/CoreGraphics")
		libCF = open("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
		libObjC := open("/usr/lib/libobjc.A.dylib")
		if frameworksErr != nil {
			return
		}
		purego.RegisterLibFunc(&autoreleasePoolPush, libObjC, "objc_autoreleasePoolPush")
		purego.RegisterLibFunc(&autoreleasePoolPop, libObjC, "objc_autoreleasePoolPop")
		purego.RegisterLibFunc(&cfRelease, libCF, "CFRelease")
		loadAX()
		loadKeyboard()
	})
	return frameworksErr
}

// withPool runs fn on a locked OS thread inside an autorelease pool, as
// Cocoa calls off the main thread need.
func withPool(fn func()) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := autoreleasePoolPush()
	defer autoreleasePoolPop(pool)
	fn()
}

var selCache sync.Map

// sel returns a selector, cached: RegisterName takes the runtime's lock.
func sel(name string) objc.SEL {
	if s, ok := selCache.Load(name); ok {
		return s.(objc.SEL)
	}
	s := objc.RegisterName(name)
	selCache.Store(name, s)
	return s
}

func class(name string) objc.ID { return objc.ID(objc.GetClass(name)) }

func send(id objc.ID, name string, args ...any) objc.ID { return id.Send(sel(name), args...) }

// nsString returns an autoreleased NSString of s.
func nsString(s string) objc.ID {
	b := []byte(s)
	if len(b) == 0 {
		return send(class("NSString"), "string")
	}
	str := send(send(class("NSString"), "alloc"), "initWithBytes:length:encoding:", unsafe.Pointer(&b[0]), len(b), 4)
	runtime.KeepAlive(b)
	return send(str, "autorelease")
}

// goString returns the text of an NSString, "" for nil.
func goString(s objc.ID) string {
	if s == 0 {
		return ""
	}
	n := objc.Send[int](s, sel("lengthOfBytesUsingEncoding:"), 4)
	if n == 0 {
		return ""
	}
	p := objc.Send[unsafe.Pointer](s, sel("UTF8String"))
	if p == nil {
		return ""
	}
	return string(unsafe.Slice((*byte)(p), n))
}

func isString(v objc.ID) bool {
	return v != 0 && objc.Send[bool](v, sel("isKindOfClass:"), class("NSString"))
}

// goBytes returns a copy of an NSData's bytes.
func goBytes(d objc.ID) []byte {
	if d == 0 {
		return nil
	}
	n := objc.Send[int](d, sel("length"))
	if n == 0 {
		return []byte{}
	}
	p := objc.Send[unsafe.Pointer](d, sel("bytes"))
	return append([]byte(nil), unsafe.Slice((*byte)(p), n)...)
}

// nsData returns an autoreleased NSData holding a copy of b.
func nsData(b []byte) objc.ID {
	if len(b) == 0 {
		return send(class("NSData"), "data")
	}
	d := send(class("NSData"), "dataWithBytes:length:", unsafe.Pointer(&b[0]), len(b))
	runtime.KeepAlive(b)
	return d
}
