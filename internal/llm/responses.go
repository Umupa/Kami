package llm

import (
	"encoding/json"
	"net/http"
)

// responses is OpenAI's Responses format, used by the OpenAI responses API and
// its compatible services.
type responses struct{}

func (responses) body(req Request) map[string]any {
	input := make([]map[string]any, 0, len(req.Messages))
	for _, m := range req.Messages {
		input = append(input, map[string]any{"role": string(m.Role), "content": m.Content})
	}
	body := map[string]any{
		"model":  req.Model.Model,
		"stream": true,
		"store":  false,
		"input":  input,
	}
	if req.System != "" {
		body["instructions"] = req.System
	}
	return body
}

func (responses) headers(http.Header) {}

func (responses) event(name, data string, onText func(string)) (bool, error) {
	if data == "" {
		return false, nil
	}
	var ev struct {
		Type     string `json:"type"`
		Delta    string `json:"delta"`
		Response struct {
			Error *struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		} `json:"response"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal([]byte(data), &ev); err != nil {
		return false, &Error{Kind: KindConfig, Detail: "unreadable stream event: " + truncate(data), Err: err}
	}
	if name == "" {
		name = ev.Type
	}
	switch name {
	case "response.output_text.delta":
		if ev.Delta != "" {
			onText(ev.Delta)
		}
	case "response.completed", "response.incomplete":
		return true, nil
	case "response.failed":
		if ev.Response.Error == nil {
			return false, &Error{Kind: KindUnknown, Detail: "response failed"}
		}
		return false, responsesError(ev.Response.Error.Code, ev.Response.Error.Message)
	case "error":
		return false, responsesError(ev.Code, ev.Message)
	}
	return false, nil
}

// responsesError classifies an error event of the Responses format.
func responsesError(code, message string) *Error {
	kind := KindUnknown
	if tooLong(message) || code == "context_length_exceeded" {
		kind = KindTooLong
	}
	return &Error{Kind: kind, Detail: message}
}
