package macos

import (
	"errors"
	"fmt"
	"image"
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
	})
	return captureErr
}

// ScreenAccess reports whether Screen Recording is granted.
func ScreenAccess() bool { return loadCapture() == nil && cgPreflightScreenCaptureAccess() }

// RequestScreenAccess asks for Screen Recording: macOS prompts the first
// time, and lists the app in System Settings after that.
func RequestScreenAccess() bool { return loadCapture() == nil && cgRequestScreenCaptureAccess() }

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

// CaptureRect captures a rectangle of the screen, in points of the global
// display space, at the display's pixel density.
func CaptureRect(r Rect, timeout time.Duration) (*Image, error) {
	if err := loadCapture(); err != nil {
		return nil, err
	}
	if !cgPreflightScreenCaptureAccess() {
		return nil, ErrNoScreenAccess
	}
	type result struct {
		ref uintptr
		err string
	}
	done := make(chan result, 1)
	block := objc.NewBlock(func(_ objc.Block, img uintptr, nserr objc.ID) {
		var res result
		if img != 0 {
			res.ref = cgImageRetain(img)
		}
		if nserr != 0 {
			res.err = goString(send(nserr, "localizedDescription"))
		}
		done <- res
	})
	defer block.Release()
	withPool(func() {
		send(class("SCScreenshotManager"), "captureImageInRect:completionHandler:", cgRect(r), block)
	})
	select {
	case res := <-done:
		if res.ref == 0 {
			return nil, fmt.Errorf("macos: capture failed: %s", res.err)
		}
		return &Image{ref: res.ref, W: cgImageGetWidth(res.ref), H: cgImageGetHeight(res.ref)}, nil
	case <-time.After(timeout):
		return nil, errors.New("macos: capture timed out")
	}
}

// Crop returns the part of img inside r, in pixels.
func (img *Image) Crop(r Rect) (*Image, error) {
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
// Lines come in reading order: top to bottom, then left to right.
func RecognizeText(img *Image) ([]Line, error) {
	if err := loadCapture(); err != nil {
		return nil, err
	}
	var lines []Line
	var err error
	withPool(func() {
		req := send(send(send(class("VNRecognizeTextRequest"), "alloc"), "init"), "autorelease")
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
		if overlap := min(a.Y+a.H, b.Y+b.H) - max(a.Y, b.Y); overlap > 0.5*min(a.H, b.H) {
			return a.X < b.X // the same row
		}
		return a.Y < b.Y
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
	if ref == 0 {
		return nil, errors.New("macos: image creation failed")
	}
	return &Image{ref: ref, W: w, H: h}, nil
}

// WarmUpVision runs one recognition on a blank image, so that the first real
// one does not pay for loading the models (about 13 seconds in Cida, then
// 0.4).
func WarmUpVision() {
	img, err := ImageFromRGBA(image.NewRGBA(image.Rect(0, 0, 32, 32)))
	if err != nil {
		return
	}
	defer img.Release()
	_, _ = RecognizeText(img)
}
