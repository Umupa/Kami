package macos

import (
	"context"
	"errors"
	"fmt"
	"image"
	"runtime"
	"sort"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// Rect is a rectangle in points or pixels, origin top left.
type Rect struct{ X, Y, W, H float64 }

type cgRect struct{ X, Y, W, H float64 }

var (
	captureOnce sync.Once
	captureErr  error

	cgPreflightScreenCaptureAccess func() bool
	cgRequestScreenCaptureAccess   func() bool
	cgImageGetWidth                func(img uintptr) int
	cgImageGetHeight               func(img uintptr) int
	cgImageRetain                  func(img uintptr) uintptr
	cgImageRelease                 func(img uintptr)
	cgImageCreateWithImageInRect   func(img uintptr, r cgRect) uintptr
	cgColorSpaceCreateDeviceRGB    func() uintptr
	cgColorSpaceRelease            func(cs uintptr)
	cgBitmapContextCreate          func(data unsafe.Pointer, w, h, bits, bytesPerRow int, cs uintptr, info uint32) uintptr
	cgContextDrawImage             func(ctx uintptr, r cgRect, img uintptr)
	cgContextRelease               func(ctx uintptr)
	cgBitmapContextCreateImage     func(ctx uintptr) uintptr
	cgContextSetRGBFillColor       func(ctx uintptr, r, g, b, a float64)
	cgContextFillRect              func(ctx uintptr, r cgRect)
	nsBeep                         func()
)

// Bitmap layout of RGBA images: premultiplied alpha last, bytes in order.
const premultipliedLast, byteOrder32Big = 1, 4 << 12

func loadCapture() error {
	captureOnce.Do(func() {
		if captureErr = openFrameworks(); captureErr != nil {
			return
		}
		for _, path := range []string{
			"/System/Library/Frameworks/ScreenCaptureKit.framework/ScreenCaptureKit",
			"/System/Library/Frameworks/Vision.framework/Vision",
		} {
			if _, err := purego.Dlopen(path, purego.RTLD_GLOBAL|purego.RTLD_NOW); err != nil {
				captureErr = err
				return
			}
		}
		purego.RegisterLibFunc(&cgPreflightScreenCaptureAccess, libCG, "CGPreflightScreenCaptureAccess")
		purego.RegisterLibFunc(&cgRequestScreenCaptureAccess, libCG, "CGRequestScreenCaptureAccess")
		purego.RegisterLibFunc(&cgImageGetWidth, libCG, "CGImageGetWidth")
		purego.RegisterLibFunc(&cgImageGetHeight, libCG, "CGImageGetHeight")
		purego.RegisterLibFunc(&cgImageRetain, libCG, "CGImageRetain")
		purego.RegisterLibFunc(&cgImageRelease, libCG, "CGImageRelease")
		purego.RegisterLibFunc(&cgImageCreateWithImageInRect, libCG, "CGImageCreateWithImageInRect")
		purego.RegisterLibFunc(&cgColorSpaceCreateDeviceRGB, libCG, "CGColorSpaceCreateDeviceRGB")
		purego.RegisterLibFunc(&cgColorSpaceRelease, libCG, "CGColorSpaceRelease")
		purego.RegisterLibFunc(&cgBitmapContextCreate, libCG, "CGBitmapContextCreate")
		purego.RegisterLibFunc(&cgContextDrawImage, libCG, "CGContextDrawImage")
		purego.RegisterLibFunc(&cgContextRelease, libCG, "CGContextRelease")
		purego.RegisterLibFunc(&cgBitmapContextCreateImage, libCG, "CGBitmapContextCreateImage")
		purego.RegisterLibFunc(&cgContextSetRGBFillColor, libCG, "CGContextSetRGBFillColor")
		purego.RegisterLibFunc(&cgContextFillRect, libCG, "CGContextFillRect")
		purego.RegisterLibFunc(&nsBeep, libAppKit, "NSBeep")
	})
	return captureErr
}

// ScreenAccess reports whether Screen Recording is granted.
func ScreenAccess() bool { return loadCapture() == nil && cgPreflightScreenCaptureAccess() }

// RequestScreenAccess asks for Screen Recording: macOS prompts the first
// time, and lists the app in System Settings after that.
func RequestScreenAccess() bool {
	if loadCapture() != nil {
		return false
	}
	if cgRequestScreenCaptureAccess() {
		return true
	}
	withPool(func() {
		url := send(class("NSURL"), "URLWithString:", nsString("x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"))
		send(send(class("NSWorkspace"), "sharedWorkspace"), "openURL:", url)
	})
	return false
}

// Image is a captured image. Release it when done.
type Image struct {
	ref  uintptr
	W, H int // pixels
}

// Release frees the image.
func (img *Image) Release() {
	if img != nil && img.ref != 0 {
		cgImageRelease(img.ref)
		img.ref = 0
	}
}

// ErrNoScreenAccess means Screen Recording has not been granted.
var ErrNoScreenAccess = errors.New("macos: Screen Recording is not granted")

// Crop returns the part of img inside r, in pixels.
func (img *Image) Crop(r Rect) (*Image, error) {
	if img == nil || img.ref == 0 || r.W <= 0 || r.H <= 0 {
		return nil, errors.New("macos: crop is empty")
	}
	ref := cgImageCreateWithImageInRect(img.ref, cgRect(r))
	if ref == 0 {
		return nil, errors.New("macos: crop is empty")
	}
	return &Image{ref: ref, W: cgImageGetWidth(ref), H: cgImageGetHeight(ref)}, nil
}

// RGBA draws img into a Go image, for showing it.
func (img *Image) RGBA() *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, img.W, img.H))
	if img.W == 0 || img.H == 0 {
		return out
	}
	cs := cgColorSpaceCreateDeviceRGB()
	defer cgColorSpaceRelease(cs)
	ctx := cgBitmapContextCreate(unsafe.Pointer(&out.Pix[0]), img.W, img.H, 8, out.Stride, cs, premultipliedLast|byteOrder32Big)
	if ctx == 0 {
		return out
	}
	defer cgContextRelease(ctx)
	cgContextDrawImage(ctx, cgRect{W: float64(img.W), H: float64(img.H)}, img.ref)
	return out
}

// Line is a line of text Vision recognized, with its box in the image as
// fractions of its size, origin top left.
type Line struct {
	Text string
	Box  Rect
}

// RecognizeText reads the text in img on this Mac, accurately, in Simplified
// Chinese and English, each line in the language Vision finds for it.
// Lines are ordered by vertical center; capture reconstructs rows and paragraphs.
func RecognizeText(img *Image) ([]Line, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return RecognizeTextContext(ctx, img)
}

// RecognizeTextContext bounds recognition and cancels Vision when the caller
// leaves. The worker owns its image until Vision has actually returned.
func RecognizeTextContext(ctx context.Context, img *Image) ([]Line, error) {
	if err := loadCapture(); err != nil {
		return nil, err
	}
	if img == nil || img.ref == 0 {
		return nil, errors.New("macos: recognition image is empty")
	}
	type result struct {
		lines []Line
		err   error
	}
	r, err := awaitNative(ctx, func(deliver func(result)) {
		owned := &Image{ref: cgImageRetain(img.ref), W: img.W, H: img.H}
		go func() {
			defer owned.Release()
			lines, err := recognizeText(ctx, owned)
			deliver(result{lines, err})
		}()
	}, func(result) {})
	if err != nil {
		return nil, fmt.Errorf("macos: text recognition: %w", err)
	}
	return r.lines, r.err
}

func recognizeText(ctx context.Context, img *Image) ([]Line, error) {
	if err := loadCapture(); err != nil {
		return nil, err
	}
	var lines []Line
	var err error
	withPool(func() {
		req := send(send(send(class("VNRecognizeTextRequest"), "alloc"), "init"), "autorelease")
		cancelDone := make(chan struct{})
		stop := context.AfterFunc(ctx, func() { send(req, "cancel"); close(cancelDone) })
		defer func() {
			if !stop() {
				<-cancelDone
			}
		}()
		if err = ctx.Err(); err != nil {
			return
		}
		send(req, "setRecognitionLevel:", 0) // accurate
		send(req, "setUsesLanguageCorrection:", true)
		langs := send(class("NSMutableArray"), "array")
		send(langs, "addObject:", nsString("zh-Hans"))
		send(langs, "addObject:", nsString("en-US"))
		send(req, "setRecognitionLanguages:", langs)
		send(req, "setAutomaticallyDetectsLanguage:", true)

		handler := send(send(send(class("VNImageRequestHandler"), "alloc"), "initWithCGImage:options:", img.ref, send(class("NSDictionary"), "dictionary")), "autorelease")
		reqs := send(class("NSArray"), "arrayWithObject:", req)
		var nserr objc.ID
		if !objc.Send[bool](handler, sel("performRequests:error:"), reqs, unsafe.Pointer(&nserr)) {
			err = fmt.Errorf("macos: text recognition failed: %s", goString(send(nserr, "localizedDescription")))
			return
		}
		results := send(req, "results")
		for i := range objc.Send[int](results, sel("count")) {
			obs := send(results, "objectAtIndex:", i)
			cands := send(obs, "topCandidates:", 1)
			if objc.Send[int](cands, sel("count")) == 0 {
				continue
			}
			text := goString(send(send(cands, "objectAtIndex:", 0), "string"))
			box := objc.Send[cgRect](obs, sel("boundingBox"))
			// Vision's boxes have their origin at the bottom left.
			lines = append(lines, Line{Text: text, Box: Rect{X: box.X, Y: 1 - box.Y - box.H, W: box.W, H: box.H}})
		}
	})
	sort.SliceStable(lines, func(i, j int) bool {
		a, b := lines[i].Box, lines[j].Box
		if a.Y+a.H/2 == b.Y+b.H/2 {
			return a.X < b.X
		}
		return a.Y+a.H/2 < b.Y+b.H/2
	})
	return lines, err
}

// ImageFromRGBA copies a Go image into an Image.
func ImageFromRGBA(src *image.RGBA) (*Image, error) {
	if err := loadCapture(); err != nil {
		return nil, err
	}
	w, h := src.Rect.Dx(), src.Rect.Dy()
	if w == 0 || h == 0 {
		return nil, errors.New("macos: empty image")
	}
	cs := cgColorSpaceCreateDeviceRGB()
	defer cgColorSpaceRelease(cs)
	buf := append([]byte(nil), src.Pix...)
	ctx := cgBitmapContextCreate(unsafe.Pointer(&buf[0]), w, h, 8, src.Stride, cs, premultipliedLast|byteOrder32Big)
	if ctx == 0 {
		return nil, errors.New("macos: bitmap context failed")
	}
	defer cgContextRelease(ctx)
	ref := cgBitmapContextCreateImage(ctx)
	runtime.KeepAlive(buf)
	if ref == 0 {
		return nil, errors.New("macos: image creation failed")
	}
	return &Image{ref: ref, W: w, H: h}, nil
}

// WarmUpVision loads both recognizers with actual Chinese and English text.
func WarmUpVision() {
	img := warmUpImage()
	if img == nil {
		return
	}
	defer img.Release()
	_, _ = RecognizeText(img)
}

func warmUpImage() *Image {
	if loadCapture() != nil {
		return nil
	}
	var img *Image
	withPool(func() {
		rep := send(send(class("NSBitmapImageRep"), "alloc"), "initWithBitmapDataPlanes:pixelsWide:pixelsHigh:bitsPerSample:samplesPerPixel:hasAlpha:isPlanar:colorSpaceName:bytesPerRow:bitsPerPixel:", uintptr(0), 480, 64, 8, 4, true, false, nsString("NSDeviceRGBColorSpace"), 0, 0)
		if rep == 0 {
			return
		}
		defer send(rep, "release")
		graphics := class("NSGraphicsContext")
		send(graphics, "saveGraphicsState")
		defer send(graphics, "restoreGraphicsState")
		send(graphics, "setCurrentContext:", send(graphics, "graphicsContextWithBitmapImageRep:", rep))
		cg := uintptr(send(send(graphics, "currentContext"), "CGContext"))
		cgContextSetRGBFillColor(cg, 1, 1, 1, 1)
		cgContextFillRect(cg, cgRect{W: 480, H: 64})
		attrs := send(class("NSMutableDictionary"), "dictionary")
		send(attrs, "setObject:forKey:", send(class("NSFont"), "systemFontOfSize:", float64(28)), nsString("NSFont"))
		send(attrs, "setObject:forKey:", send(class("NSColor"), "blackColor"), nsString("NSColor"))
		send(nsString("截图翻译 Gloss"), "drawAtPoint:withAttributes:", struct{ X, Y float64 }{16, 20}, attrs)
		ref := uintptr(send(rep, "CGImage"))
		if ref != 0 {
			img = &Image{ref: cgImageRetain(ref), W: 480, H: 64}
		}
	})
	return img
}

// CaptureFailureFeedback matches the capture shortcut's nonmodal failure cue.
func CaptureFailureFeedback() {
	if loadCapture() == nil {
		nsBeep()
	}
}
