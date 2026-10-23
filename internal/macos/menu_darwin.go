package macos

import (
	"fmt"

	"github.com/ebitengine/purego/objc"
)

// InstallMenuItemSymbol gives newly created native menu items with title a
// template SF Symbol. Call before creating the tray, on the main thread; call
// the returned cleanup on the same thread when the app quits.
// MyGo does not expose menu item images or native menu handles.
func InstallMenuItemSymbol(title, symbol string) (func(), error) {
	if err := openFrameworks(); err != nil {
		return nil, err
	}
	var observer, image objc.ID
	withPool(func() {
		image = send(class("NSImage"), "imageWithSystemSymbolName:accessibilityDescription:", nsString(symbol), 0)
		if image == 0 {
			return
		}
		send(image, "retain")
		send(image, "setTemplate:", true)
		block := objc.NewBlock(func(_ objc.Block, notification objc.ID) {
			menu := send(notification, "object")
			item := send(menu, "itemWithTitle:", nsString(title))
			if item != 0 {
				send(item, "setImage:", image)
			}
		})
		defer block.Release()
		observer = send(send(class("NSNotificationCenter"), "defaultCenter"),
			"addObserverForName:object:queue:usingBlock:", nsString("NSMenuDidAddItemNotification"), 0, 0, block)
		send(observer, "retain")
	})
	if image == 0 {
		return nil, fmt.Errorf("menu symbol %q is unavailable", symbol)
	}
	return func() {
		withPool(func() {
			send(send(class("NSNotificationCenter"), "defaultCenter"), "removeObserver:", observer)
			send(observer, "release")
			send(image, "release")
		})
	}, nil
}

// Tint is the color of a status symbol in a menu.
type Tint int

const (
	// TintNone draws the symbol as a template, in the menu's text color.
	TintNone Tint = iota
	TintGreen
	TintOrange
	TintRed
)

// StatusMenuItem is a native menu item whose SF Symbol changes after MyGo
// created it, as a status line does. It is found by the title it is created
// with; MyGo changes the title later, never the image.
type StatusMenuItem struct {
	item     objc.ID
	symbol   string
	tint     Tint
	observer objc.ID
	images   map[string]objc.ID
}

// WatchStatusMenuItem finds the native items created with title, the latest
// when the menu is rebuilt. Call before creating the tray, on the main
// thread; call Close on the same thread when the app quits.
func WatchStatusMenuItem(title string) (*StatusMenuItem, error) {
	if err := openFrameworks(); err != nil {
		return nil, err
	}
	s := &StatusMenuItem{images: map[string]objc.ID{}}
	withPool(func() {
		block := objc.NewBlock(func(_ objc.Block, notification objc.ID) {
			item := send(send(notification, "object"), "itemWithTitle:", nsString(title))
			if item == 0 || item == s.item {
				return
			}
			send(item, "retain")
			if s.item != 0 {
				send(s.item, "release")
			}
			s.item = item
			s.apply()
		})
		defer block.Release()
		s.observer = send(send(class("NSNotificationCenter"), "defaultCenter"),
			"addObserverForName:object:queue:usingBlock:", nsString("NSMenuDidAddItemNotification"), 0, 0, block)
		send(s.observer, "retain")
	})
	return s, nil
}

// SetSymbol shows an SF Symbol in tint, now or once the item exists. Main
// thread only.
func (s *StatusMenuItem) SetSymbol(symbol string, tint Tint) {
	s.symbol, s.tint = symbol, tint
	withPool(s.apply)
}

func (s *StatusMenuItem) apply() {
	if s.item == 0 || s.symbol == "" {
		return
	}
	send(s.item, "setImage:", s.image())
}

// image is the symbol in its tint, made once per pair.
func (s *StatusMenuItem) image() objc.ID {
	key := fmt.Sprintf("%s/%d", s.symbol, s.tint)
	if img, ok := s.images[key]; ok {
		return img
	}
	img := send(class("NSImage"), "imageWithSystemSymbolName:accessibilityDescription:", nsString(s.symbol), 0)
	if img == 0 {
		return 0
	}
	if color := tintColor(s.tint); color != 0 {
		conf := send(class("NSImageSymbolConfiguration"), "configurationWithHierarchicalColor:", color)
		img = send(img, "imageWithSymbolConfiguration:", conf)
	} else {
		send(img, "setTemplate:", true)
	}
	send(img, "retain")
	s.images[key] = img
	return img
}

func tintColor(t Tint) objc.ID {
	switch t {
	case TintGreen:
		return send(class("NSColor"), "systemGreenColor")
	case TintOrange:
		return send(class("NSColor"), "systemOrangeColor")
	case TintRed:
		return send(class("NSColor"), "systemRedColor")
	}
	return 0
}

// Close stops watching and releases the item. Main thread only.
func (s *StatusMenuItem) Close() {
	withPool(func() {
		send(send(class("NSNotificationCenter"), "defaultCenter"), "removeObserver:", s.observer)
		send(s.observer, "release")
		if s.item != 0 {
			send(s.item, "release")
		}
		for _, img := range s.images {
			send(img, "release")
		}
	})
}
