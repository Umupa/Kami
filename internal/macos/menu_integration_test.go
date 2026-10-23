//go:build integration

package macos

import (
	"runtime"
	"testing"

	"github.com/ebitengine/purego/objc"
)

// These menus stay offscreen; no status item or application window is created.
func TestMenuItemSymbol(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const title = "重新启动 Kami"
	cleanup, err := InstallMenuItemSymbol(title, "arrow.clockwise")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	withPool(func() {
		for range 2 { // The icon must also survive a rebuilt tray menu.
			menu := send(send(class("NSMenu"), "alloc"), "initWithTitle:", nsString("Kami"))
			defer send(menu, "release")
			for _, label := range []string{"显示面板", title, "退出 Kami"} {
				item := send(send(class("NSMenuItem"), "alloc"), "initWithTitle:action:keyEquivalent:", nsString(label), 0, nsString(""))
				send(menu, "addItem:", item)
				send(item, "release")
				icon := send(item, "image")
				if label != title {
					if icon != 0 {
						t.Errorf("unexpected icon on %q", label)
					}
					continue
				}
				if icon == 0 {
					t.Fatal("restart menu item has no image")
				}
				if !objc.Send[bool](icon, sel("isTemplate")) {
					t.Error("restart icon does not follow the system appearance")
				}
				if len(goBytes(send(icon, "TIFFRepresentation"))) == 0 {
					t.Error("restart icon has no rendered pixels")
				}
			}
		}
	})
}

func TestDockActionMenuSymbol(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for _, title := range []string{"显示 Dock 图标", "隐藏 Dock 图标"} {
		cleanup, err := InstallMenuItemSymbol(title, "dock.rectangle")
		if err != nil {
			t.Fatal(err)
		}
		defer cleanup()
	}
	withPool(func() {
		// Either title can be present when MyGo rebuilds the menu.
		for _, title := range []string{"显示 Dock 图标", "隐藏 Dock 图标"} {
			menu := send(send(class("NSMenu"), "alloc"), "initWithTitle:", nsString("Kami"))
			defer send(menu, "release")
			item := send(send(class("NSMenuItem"), "alloc"), "initWithTitle:action:keyEquivalent:", nsString(title), 0, nsString(""))
			send(menu, "addItem:", item)
			send(item, "release")
			icon := send(item, "image")
			if icon == 0 || !objc.Send[bool](icon, sel("isTemplate")) {
				t.Fatal("Dock action has no template image in the menu icon column")
			}
			send(item, "setTitle:", nsString("另一种显示状态"))
			if send(item, "image") != icon || objc.Send[int](item, sel("state")) != 0 {
				t.Fatal("changing the action lost its image or introduced a checkmark")
			}
		}
	})
}

func TestStatusMenuItem(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	const title = "Kami 状态"
	s, err := WatchStatusMenuItem(title)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	s.SetSymbol("circle.dashed", TintNone) // before the item exists
	withPool(func() {
		for range 2 { // The latest of a rebuilt tray menu is the one shown.
			menu := send(send(class("NSMenu"), "alloc"), "initWithTitle:", nsString("Kami"))
			defer send(menu, "release")
			item := send(send(class("NSMenuItem"), "alloc"), "initWithTitle:action:keyEquivalent:", nsString(title), 0, nsString(""))
			send(menu, "addItem:", item)
			send(item, "release")
			icon := send(item, "image")
			if icon == 0 || !objc.Send[bool](icon, sel("isTemplate")) {
				t.Fatal("a pending untinted symbol is not shown as a template")
			}
			// MyGo retitles the item; the symbol follows SetSymbol alone.
			send(item, "setTitle:", nsString("deepseek-chat"))
			s.SetSymbol("checkmark.circle.fill", TintGreen)
			icon = send(item, "image")
			if icon == 0 || objc.Send[bool](icon, sel("isTemplate")) {
				t.Fatal("a tinted symbol is missing or drawn as a template")
			}
			if len(goBytes(send(icon, "TIFFRepresentation"))) == 0 {
				t.Error("the tinted symbol has no rendered pixels")
			}
			s.SetSymbol("circle.dashed", TintNone)
		}
	})
}
