package llm

import (
	"encoding/json"
	"net/http"
)

// anthropicMessages is Anthropic's Messages format.
type anthropicMessages struct{}

func (anthropicMessages) body(req Request) map[string]any {
	messages := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		messages = append(messages, map[string]any{"role": string(m.Role), "content": m.Content})
	}
	body := map[string]any{
		"model":      req.Model.Model,
		"stream":     true,
		"max_tokens": 8192,
		"messages":   messages,
	}
	if req.System != "" {
		body["system"] = req.System
	}
	return body
}

func (anthropicMessages) headers(h http.Header) {
	h.Set("anthropic-version", "2023-06-01")
}

func (anthropicMessages) event(name, data string, onText func(string)) (bool, error) {
	if data == "" {
		return false, nil
	}
	var ev struct {
		Type  string `json:"type"`
		Delta struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"delta"`
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return false, &Error{Kind: KindConfig, Detail: "unreadable stream event: " + truncate(data), Err: err}
	}
	if name == "" {
		name = ev.Type
	}
	switch name {
	case "content_block_delta":
		if ev.Delta.Type == "text_delta" && ev.Delta.Text != "" {
			onText(ev.Delta.Text)
		}
	case "message_stop":
		return true, nil
	case "error":
		if ev.Error == nil {
			return false, &Error{Kind: KindUnknown, Detail: "stream error"}
		}
		return false, anthropicError(ev.Error.Type, ev.Error.Message)
	}
	return false, nil
}

// anthropicError classifies an error event of the Messages format.
func anthropicError(typ, message string) *Error {
	kind := KindUnknown
	switch {
	case typ == "overloaded_error" || typ == "rate_limit_error":
		kind = KindLimited
	case tooLong(message):
		kind = KindTooLong
	case typ == "invalid_request_error" || typ == "authentication_error" ||
		typ == "permission_error" || typ == "not_found_error":
		kind = KindConfig
	}
	return &Error{Kind: kind, Detail: message}
}
