package llm

import (
	"encoding/json"
	"net/http"
)

// chatCompletions is OpenAI's Chat Completions format, which most services
// and local servers speak.
type chatCompletions struct{}

func (chatCompletions) body(req Request) map[string]any {
	msgs := make([]map[string]any, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, map[string]any{"role": string(m.Role), "content": m.Content})
	}
	return map[string]any{
		"model":    req.Model.Model,
		"messages": msgs,
		"stream":   true,
	}
}

func (chatCompletions) headers(http.Header) {}

func (chatCompletions) event(_, data string, onText func(string)) (bool, error) {
	switch data {
	case "":
		return false, nil
	case "[DONE]":
		return true, nil
	}
	var chunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
			FinishReason *string `json:"finish_reason"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return false, &Error{Kind: KindConfig, Detail: "unreadable stream event: " + truncate(data), Err: err}
	}
	if chunk.Error != nil {
		kind := KindUnknown
		if tooLong(chunk.Error.Message) {
			kind = KindTooLong
		}
		return false, &Error{Kind: kind, Detail: chunk.Error.Message}
	}
	for _, c := range chunk.Choices {
		if c.Delta.Content != "" {
			onText(c.Delta.Content)
		}
	}
	return false, nil
}

// truncate keeps diagnostics short.
func truncate(s string) string {
	const n = 300
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
