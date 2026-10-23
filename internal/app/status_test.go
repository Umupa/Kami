package app

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/llm"
	"github.com/Umupa/Kami/internal/macos"
)

func TestMenuStatus(t *testing.T) {
	m := &config.Model{Endpoint: "https://api.deepseek.com/chat/completions", Format: config.FormatChatCompletions, Model: "deepseek-chat", APIKey: "secret-key"}
	noKey := *m
	noKey.APIKey = ""
	local := noKey
	local.Auth = config.AuthNone
	for _, tc := range []struct {
		name    string
		model   *config.Model
		conn    connection
		want    statusLine
		tooltip string
	}{
		{"no model", nil, connection{}, statusLine{"未连接模型", "circle.dashed", macos.TintNone}, "Kami · 未连接模型"},
		{"no key", &noKey, connection{}, statusLine{"deepseek-chat · 缺少 API Key", "xmark.circle.fill", macos.TintRed}, "缺少 API Key"},
		{"checking", m, connection{}, statusLine{"deepseek-chat", "circle.dotted", macos.TintNone}, "（api.deepseek.com）正在检查连接"},
		{"local, no key needed", &local, connection{known: true}, statusLine{"deepseek-chat", "checkmark.circle.fill", macos.TintGreen}, "连接正常"},
		{"answered", m, connection{known: true}, statusLine{"deepseek-chat", "checkmark.circle.fill", macos.TintGreen}, "连接正常"},
		{"offline", m, connection{known: true, err: &llm.Error{Kind: llm.KindOffline}}, statusLine{"deepseek-chat · 连不上", "xmark.circle.fill", macos.TintRed}, "连不上"},
		{"rejected", m, connection{known: true, err: &llm.Error{Kind: llm.KindConfig, Status: 401}}, statusLine{"deepseek-chat · 请求被拒绝", "xmark.circle.fill", macos.TintRed}, "请求被拒绝"},
		{"limited", m, connection{known: true, err: &llm.Error{Kind: llm.KindLimited}}, statusLine{"deepseek-chat · 请求受限", "exclamationmark.circle.fill", macos.TintOrange}, "请求受限"},
		{"check timed out", m, connection{known: true, err: fmt.Errorf("ping: %w", context.DeadlineExceeded)}, statusLine{"deepseek-chat · 响应超时", "exclamationmark.circle.fill", macos.TintOrange}, "响应超时"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, broken, tooltip := menuStatus(config.Snapshot{Config: &config.Config{Model: tc.model}}, tc.conn)
			if got != tc.want || broken != nil {
				t.Errorf("status %+v, broken %+v; want %+v and no broken line", got, broken, tc.want)
			}
			if !strings.Contains(tooltip, tc.tooltip) || strings.Contains(tooltip, "secret-key") {
				t.Errorf("tooltip %q: want it to say %q, never the key", tooltip, tc.tooltip)
			}
		})
	}
}

func TestMenuStatusBrokenConfig(t *testing.T) {
	problems := &config.Error{Problems: []config.Problem{{Path: "defaultModel", Message: "未知字段"}, {Path: "models", Message: "未知字段"}}}
	_, broken, tooltip := menuStatus(config.Snapshot{Config: &config.Config{}, Err: problems}, connection{})
	if want := (statusLine{"config.json 有 2 处错误", "exclamationmark.triangle.fill", macos.TintOrange}); broken == nil || *broken != want {
		t.Fatalf("broken line %+v, want %+v", broken, want)
	}
	if !strings.Contains(tooltip, "defaultModel") {
		t.Errorf("tooltip %q does not say what is wrong", tooltip)
	}
}

func TestTellsConnection(t *testing.T) {
	if tellsConnection(&llm.Error{Kind: llm.KindTooLong}) {
		t.Error("a request too long for the model changed the connection")
	}
	if !tellsConnection(nil) || !tellsConnection(&llm.Error{Kind: llm.KindOffline}) {
		t.Error("an answer or a failed connection did not change it")
	}
}
