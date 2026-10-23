package app

import (
	"context"
	"errors"
	"net/url"
	"strconv"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/llm"
	"github.com/Umupa/Kami/internal/macos"
)

// connection is what Kami knows of the model's connection: from a check when
// the model is configured, then from every request the user sends.
type connection struct {
	// model is the model the rest is about.
	model *config.Model
	known bool
	// err is how the last check or request failed; nil when it was answered.
	err error
}

// statusLine is a line at the top of the menu: an SF Symbol and a label.
type statusLine struct {
	label  string
	symbol string
	tint   macos.Tint
}

// menuStatus is what the menu says about Kami: the model and how its
// connection is, a line for a broken config.json when there is one, and the
// tray's tooltip.
func menuStatus(s config.Snapshot, c connection) (model statusLine, broken *statusLine, tooltip string) {
	m := s.Config.Model
	switch {
	case m == nil:
		model = statusLine{"未连接模型", "circle.dashed", macos.TintNone}
		tooltip = "Kami · 未连接模型"
	case m.APIKey == "" && m.Auth != config.AuthNone:
		model = statusLine{m.Model + " · 缺少 API Key", "xmark.circle.fill", macos.TintRed}
		tooltip = "Kami · " + m.Model + "（" + host(m.Endpoint) + "）缺少 API Key"
	case !c.known:
		model = statusLine{m.Model, "circle.dotted", macos.TintNone}
		tooltip = "Kami · " + m.Model + "（" + host(m.Endpoint) + "）正在检查连接"
	case c.err == nil:
		model = statusLine{m.Model, "checkmark.circle.fill", macos.TintGreen}
		tooltip = "Kami · " + m.Model + "（" + host(m.Endpoint) + "）连接正常"
	default:
		problem, tint := failure(c.err)
		model = statusLine{m.Model + " · " + problem, "xmark.circle.fill", tint}
		if tint == macos.TintOrange {
			model.symbol = "exclamationmark.circle.fill"
		}
		tooltip = "Kami · " + m.Model + "（" + host(m.Endpoint) + "）" + problem
	}
	if s.Err != nil {
		label := "config.json 有错误"
		var ce *config.Error
		if errors.As(s.Err, &ce) && len(ce.Problems) > 1 {
			label = "config.json 有 " + strconv.Itoa(len(ce.Problems)) + " 处错误"
		}
		broken = &statusLine{label, "exclamationmark.triangle.fill", macos.TintOrange}
		tooltip += "\n" + s.Err.Error()
	}
	return model, broken, tooltip
}

// failure names a failed request in a few words, orange when it is likely
// to pass by itself.
func failure(err error) (string, macos.Tint) {
	var le *llm.Error
	if !errors.As(err, &le) {
		if errors.Is(err, context.DeadlineExceeded) {
			return "响应超时", macos.TintOrange
		}
		return "请求失败", macos.TintRed
	}
	switch le.Kind {
	case llm.KindTimeout:
		return "响应超时", macos.TintOrange
	case llm.KindLimited:
		return "请求受限", macos.TintOrange
	case llm.KindOffline:
		return "连不上", macos.TintRed
	case llm.KindTLS:
		return "安全连接失败", macos.TintRed
	case llm.KindConfig:
		return "请求被拒绝", macos.TintRed
	}
	return "请求失败", macos.TintRed
}

// tellsConnection reports whether how a request ended says anything about
// the connection: one too long for the model does not.
func tellsConnection(err error) bool {
	var le *llm.Error
	return !errors.As(err, &le) || le.Kind != llm.KindTooLong
}

func host(endpoint string) string {
	if u, err := url.Parse(endpoint); err == nil && u.Host != "" {
		return u.Host
	}
	return endpoint
}
