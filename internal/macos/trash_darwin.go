package macos

import (
	"errors"
	"time"

	"github.com/ebitengine/purego/objc"
)

// trashTimeout bounds moving a file to the Trash, which may wait on a
// file provider such as iCloud Drive.
const trashTimeout = 3 * time.Second

// Trash moves a file to the Trash, where the user can get it back.
func Trash(path string) error {
	if err := openFrameworks(); err != nil {
		return err
	}
	done := make(chan bool, 1)
	go withPool(func() {
		url := send(class("NSURL"), "fileURLWithPath:", nsString(path))
		fm := send(class("NSFileManager"), "defaultManager")
		done <- objc.Send[bool](fm, sel("trashItemAtURL:resultingItemURL:error:"), url, uintptr(0), uintptr(0))
	})
	select {
	case ok := <-done:
		if !ok {
			return errors.New("macos: the file could not be moved to the Trash")
		}
		return nil
	case <-time.After(trashTimeout):
		return errors.New("macos: moving to the Trash timed out")
	}
}
