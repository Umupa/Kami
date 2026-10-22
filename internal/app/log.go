package app

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// maxLogSize is how large the log grows before the next launch starts it over.
const maxLogSize = 4 << 20

// openLog returns the app's logger, writing to dir/Kami.log and, under
// development builds, to stderr too. Log records carry events and timings,
// never text the user selected, typed or received.
func openLog(dir string, dev bool) *slog.Logger {
	var out []io.Writer
	if dev {
		out = append(out, os.Stderr)
	}
	path := filepath.Join(dir, "Kami.log")
	flags := os.O_CREATE | os.O_WRONLY | os.O_APPEND
	if fi, err := os.Stat(path); err == nil && fi.Size() > maxLogSize {
		flags |= os.O_TRUNC
	}
	if f, err := os.OpenFile(path, flags, 0o600); err == nil {
		out = append(out, f)
	}
	if len(out) == 0 {
		return slog.New(slog.DiscardHandler)
	}
	return slog.New(slog.NewTextHandler(io.MultiWriter(out...), nil))
}
