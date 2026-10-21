package macos

import (
	"context"
	"errors"
	"fmt"

	"github.com/ebitengine/purego/objc"
)

// CaptureDisplay captures the full display at its native pixel size without
// the pointer. Display identity avoids AppKit/global-rectangle conversions.
func CaptureDisplay(ctx context.Context, displayID uint32, width, height int) (*Image, error) {
	if err := loadCapture(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if width <= 0 || height <= 0 {
		return nil, errors.New("macos: display has no pixels")
	}
	if !cgPreflightScreenCaptureAccess() {
		return nil, ErrNoScreenAccess
	}
	type displayResult struct {
		display objc.ID
		err     error
	}
	d, err := awaitNative(ctx, func(deliver func(displayResult)) {
		block := objc.NewBlock(func(_ objc.Block, content, nserr objc.ID) {
			withPool(func() {
				var r displayResult
				if nserr != 0 {
					r.err = fmt.Errorf("macos: display enumeration failed: %s", goString(send(nserr, "localizedDescription")))
				} else {
					displays := send(content, "displays")
					for i := range objc.Send[int](displays, sel("count")) {
						d := send(displays, "objectAtIndex:", i)
						if objc.Send[uint32](d, sel("displayID")) == displayID {
							r.display = send(d, "retain")
							break
						}
					}
					if r.display == 0 {
						r.err = errors.New("macos: capture display is unavailable")
					}
				}
				deliver(r)
			})
		})
		defer block.Release()
		withPool(func() {
			send(class("SCShareableContent"), "getShareableContentExcludingDesktopWindows:onScreenWindowsOnly:completionHandler:", false, true, block)
		})
	}, func(r displayResult) {
		if r.display != 0 {
			send(r.display, "release")
		}
	})
	if err != nil {
		return nil, fmt.Errorf("macos: display enumeration: %w", err)
	}
	if d.err != nil {
		return nil, d.err
	}
	defer send(d.display, "release")
	type imageResult struct {
		image *Image
		err   error
	}
	r, err := awaitNative(ctx, func(deliver func(imageResult)) {
		block := objc.NewBlock(func(_ objc.Block, img uintptr, nserr objc.ID) {
			var r imageResult
			if img != 0 {
				r.image = &Image{ref: cgImageRetain(img), W: cgImageGetWidth(img), H: cgImageGetHeight(img)}
			}
			if nserr != 0 {
				r.err = fmt.Errorf("macos: capture failed: %s", goString(send(nserr, "localizedDescription")))
			}
			if r.image == nil && r.err == nil {
				r.err = errors.New("macos: capture returned no image")
			}
			deliver(r)
		})
		defer block.Release()
		withPool(func() {
			filter := send(send(send(class("SCContentFilter"), "alloc"), "initWithDisplay:excludingWindows:", d.display, send(class("NSArray"), "array")), "autorelease")
			config := send(send(send(class("SCStreamConfiguration"), "alloc"), "init"), "autorelease")
			send(config, "setWidth:", width)
			send(config, "setHeight:", height)
			send(config, "setShowsCursor:", false)
			send(config, "setCaptureResolution:", 1) // SCCaptureResolutionBest
			send(class("SCScreenshotManager"), "captureImageWithFilter:configuration:completionHandler:", filter, config, block)
		})
	}, func(r imageResult) { r.image.Release() })
	if err != nil {
		return nil, fmt.Errorf("macos: screen capture: %w", err)
	}
	if r.err != nil {
		r.image.Release()
		return nil, r.err
	}
	return r.image, nil
}

// ConfigureCaptureWindow gives a nonactivating MyGo panel Cida's capture
// window behavior. It runs on the main thread before the window is shown.
func ConfigureCaptureWindow(handle uintptr) error {
	if handle == 0 {
		return errors.New("macos: capture window is unavailable")
	}
	if err := openFrameworks(); err != nil {
		return err
	}
	w := objc.ID(handle)
	// MyGo's Frameless keeps a titled, full-size-content window. A capture
	// must have no native titlebar or rounded frame over the frozen display.
	// The nonactivating bit was already present when MyGo created the panel.
	send(w, "setStyleMask:", 1<<7)                                 // borderless, NSWindowStyleMaskNonactivatingPanel
	send(w, "setLevel:", 1000)                                     // kCGScreenSaverWindowLevel
	send(w, "setCollectionBehavior:", (1<<0)|(1<<4)|(1<<6)|(1<<8)) // all Spaces, stationary, ignores cycle, full-screen auxiliary
	send(w, "setHidesOnDeactivate:", false)
	send(w, "setHasShadow:", false)
	send(w, "setOpaque:", true)
	send(w, "setAnimationBehavior:", 2) // NSWindowAnimationBehaviorNone
	return nil
}
