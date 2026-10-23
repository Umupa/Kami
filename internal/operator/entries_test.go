package operator

import (
	"slices"
	"strings"
	"testing"

	"github.com/Umupa/Kami/internal/config"
)

func TestEntriesNeedPermissions(t *testing.T) {
	c, err := config.Decode([]byte(`{"version":1,"shortcuts":{"panel":"double-command","capture":"none"}}`))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]string{}
	for _, e := range Entries(c) {
		got[e.ID] = e.Needs
	}
	want := map[string][]string{
		"panel":     {PermInputMonitoring},
		"selection": {PermInputMonitoring, PermAccessibility},
	}
	if len(got) != len(want) {
		t.Fatalf("entries %v, want %v: a cleared shortcut is no entry", got, want)
	}
	for id, needs := range want {
		if !slices.Equal(got[id], needs) {
			t.Errorf("%s needs %v, want %v", id, got[id], needs)
		}
	}
}

func TestGuideListsEntries(t *testing.T) {
	c, err := config.Decode([]byte(`{"version":1}`))
	if err != nil {
		t.Fatal(err)
	}
	guide := Guide(State{Config: c, Permissions: []Permission{
		{ID: PermAccessibility, Granted: true},
		{ID: PermInputMonitoring, Granted: true, Restart: true},
		{ID: PermScreenRecording},
	}})
	for _, want := range []string{
		"| ⌥Space | 打开面板 | 不需要 |",
		"| 双击 ⌥ | 带入选中的文字，立即翻译 | 输入监控 重启后生效 · 辅助功能 ✓ |",
		"| ⌥S | 截图识别文字，立即翻译 | 屏幕录制 未授权 |",
	} {
		if !strings.Contains(guide, want) {
			t.Errorf("guide lacks %q:\n%s", want, guide)
		}
	}
	if strings.Contains(guide, "截图识别改成 ⌥S") {
		t.Error("the guide suggests the default shortcut as a change")
	}
}
