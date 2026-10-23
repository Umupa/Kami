package app

import (
	"testing"

	"github.com/Umupa/Kami/internal/panel"
)

func TestUpdateOffered(t *testing.T) {
	available := panel.UpdateState{Phase: panel.UpdateAvailable, Version: "0.2.0"}
	for _, tc := range []struct {
		name    string
		s       panel.UpdateState
		later   bool
		skipped string
		want    bool
	}{
		{"none", panel.UpdateState{}, false, "", false},
		{"available", available, false, "", true},
		{"put off", available, true, "", false},
		{"skipped", available, false, "0.2.0", false},
		{"an older skip", available, false, "0.1.5", true},
		// Once the user chose to install it, it is theirs to finish.
		{"downloading, skipped before", panel.UpdateState{Phase: panel.UpdateDownloading, Version: "0.2.0"}, false, "0.2.0", true},
		{"ready", panel.UpdateState{Phase: panel.UpdateReady, Version: "0.2.0"}, false, "", true},
		{"ready, put off", panel.UpdateState{Phase: panel.UpdateReady, Version: "0.2.0"}, true, "", false},
	} {
		if got := offered(tc.s, tc.later, tc.skipped); got != tc.want {
			t.Errorf("%s: offered = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestUpdateLabel(t *testing.T) {
	for _, tc := range []struct {
		s       panel.UpdateState
		skipped string
		want    string
	}{
		{panel.UpdateState{}, "", ""},
		{panel.UpdateState{Phase: panel.UpdateAvailable, Version: "0.2.0"}, "", "更新到 0.2.0…"},
		{panel.UpdateState{Phase: panel.UpdateAvailable, Version: "0.2.0"}, "0.2.0", ""},
		{panel.UpdateState{Phase: panel.UpdateDownloading, Progress: -1}, "", "正在下载更新…"},
		{panel.UpdateState{Phase: panel.UpdateDownloading, Progress: 0.425}, "", "正在下载更新 42%"},
		{panel.UpdateState{Phase: panel.UpdateReady}, "", "重启以完成更新"},
		{panel.UpdateState{Phase: panel.UpdateFailed, Version: "0.2.0"}, "", "更新失败，重试…"},
	} {
		if got := updateLabel(tc.s, tc.skipped); got != tc.want {
			t.Errorf("updateLabel(%+v, %q) = %q, want %q", tc.s, tc.skipped, got, tc.want)
		}
	}
}
