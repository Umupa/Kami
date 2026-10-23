package operator

import (
	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/mode"
)

// The system permissions the app uses, by the ids actions and Permission
// carry.
const (
	PermAccessibility   = "accessibility"
	PermInputMonitoring = "input-monitoring"
	PermScreenRecording = "screen-recording"
)

// PermissionName is a permission's name as System Settings shows it.
func PermissionName(id string) string {
	return map[string]string{
		PermAccessibility:   "辅助功能",
		PermInputMonitoring: "输入监控",
		PermScreenRecording: "屏幕录制",
	}[id]
}

// Entry is one of the global entry points: its shortcut, what it does, and
// the permissions it needs to work.
type Entry struct {
	// ID is its field under "shortcuts" in config.json.
	ID       string
	Does     string
	Shortcut config.Shortcut
	// Needs are permission ids, in the order the user grants them: a
	// double tap is not seen at all without Input Monitoring.
	Needs []string
}

// Entries returns the entry points c gives a shortcut, in the order the
// panel lists them.
func Entries(c *config.Config) []Entry {
	first := "执行"
	if modes, err := mode.Resolve(c); err == nil && len(modes) > 0 {
		first = modes[0].Name
	}
	all := []struct {
		id, does, value string
		needs           []string
	}{
		{"panel", "打开面板", c.Shortcuts.Panel, nil},
		{"selection", "带入选中的文字，立即" + first, c.Shortcuts.Selection, []string{PermAccessibility}},
		{"capture", "截图识别文字，立即翻译", c.Shortcuts.Capture, []string{PermScreenRecording}},
	}
	var entries []Entry
	for _, e := range all {
		sc, err := config.ParseShortcut(e.value)
		if err != nil || sc.IsZero() {
			continue
		}
		needs := e.needs
		if sc.Double != 0 {
			needs = append([]string{PermInputMonitoring}, needs...)
		}
		entries = append(entries, Entry{ID: e.id, Does: e.does, Shortcut: sc, Needs: needs})
	}
	return entries
}
