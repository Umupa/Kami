package llm

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Umupa/Kami/internal/config"
)

func TestAnthropicMessagesRequest(t *testing.T) {
	t.Run("with system", func(t *testing.T) {
		srv, recCh := captureServer(t)
		model := config.Model{
			Endpoint: srv.URL + "/v1/messages",
			Format:   config.FormatAnthropicMessages,
			Model:    "claude-test",
			Auth:     config.AuthXAPIKey,
			Headers:  map[string]string{"X-Trace": "trace-1"},
		}
		err := (HTTP{Client: srv.Client()}).Stream(context.Background(), Request{
			Model:  model,
			Key:    "secret",
			System: "be brief",
			Messages: []Message{
				{Role: RoleUser, Content: "hi"},
				{Role: RoleAssistant, Content: "hello"},
			},
		}, func(string) {})
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		rec := <-recCh
		checkRequestBasics(t, rec, "/v1/messages")
		if got := rec.header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q, want 2023-06-01", got)
		}
		if got := rec.header.Get("X-Api-Key"); got != "secret" {
			t.Errorf("X-Api-Key = %q, want secret", got)
		}
		if got := rec.header.Get("X-Trace"); got != "trace-1" {
			t.Errorf("X-Trace = %q, want trace-1", got)
		}
		want := map[string]any{
			"model":      "claude-test",
			"stream":     true,
			"max_tokens": float64(8192),
			"system":     "be brief",
			"messages": []any{
				map[string]any{"role": "user", "content": "hi"},
				map[string]any{"role": "assistant", "content": "hello"},
			},
		}
		if body := decodeRequestBody(t, rec); !reflect.DeepEqual(body, want) {
			t.Errorf("body = %#v, want %#v", body, want)
		}
	})

	t.Run("without system", func(t *testing.T) {
		srv, recCh := captureServer(t)
		model := config.Model{
			Endpoint: srv.URL + "/v1/messages",
			Format:   config.FormatAnthropicMessages,
			Model:    "claude-test",
		}
		err := (HTTP{Client: srv.Client()}).Stream(context.Background(), Request{
			Model:    model,
			Messages: []Message{{Role: RoleUser, Content: "hi"}},
		}, func(string) {})
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		rec := <-recCh
		checkRequestBasics(t, rec, "/v1/messages")
		body := decodeRequestBody(t, rec)
		if _, ok := body["system"]; ok {
			t.Errorf("system = %v, want it omitted", body["system"])
		}
		want := map[string]any{
			"model":      "claude-test",
			"stream":     true,
			"max_tokens": float64(8192),
			"messages": []any{
				map[string]any{"role": "user", "content": "hi"},
			},
		}
		if !reflect.DeepEqual(body, want) {
			t.Errorf("body = %#v, want %#v", body, want)
		}
	})

	t.Run("model header overrides anthropic-version", func(t *testing.T) {
		srv, recCh := captureServer(t)
		model := config.Model{
			Endpoint: srv.URL + "/v1/messages",
			Format:   config.FormatAnthropicMessages,
			Model:    "claude-test",
			Headers:  map[string]string{"Anthropic-Version": "2024-01-01"},
		}
		err := (HTTP{Client: srv.Client()}).Stream(context.Background(),
			Request{Model: model, Messages: []Message{{Role: RoleUser, Content: "hi"}}}, func(string) {})
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		rec := <-recCh
		if got := rec.header.Get("anthropic-version"); got != "2024-01-01" {
			t.Errorf("anthropic-version = %q, want the model's 2024-01-01", got)
		}
	})
}

func TestAnthropicMessagesAuth(t *testing.T) {
	testAuthHeaders(t, config.FormatAnthropicMessages)
}

func TestAnthropicMessagesBodyMerge(t *testing.T) {
	srv, recCh := captureServer(t)
	model := config.Model{
		Endpoint: srv.URL + "/v1/messages",
		Format:   config.FormatAnthropicMessages,
		Model:    "claude-test",
		Body: map[string]any{
			"model":       "merged-model",
			"max_tokens":  nil,
			"stream":      nil,
			"metadata":    map[string]any{"user_id": "u-1"},
			"temperature": 0.4,
		},
	}
	err := (HTTP{Client: srv.Client()}).Stream(context.Background(), Request{
		Model:    model,
		System:   "be brief",
		Messages: []Message{{Role: RoleUser, Content: "hi"}},
	}, func(string) {})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	body := decodeRequestBody(t, <-recCh)
	if got := body["model"]; got != "merged-model" {
		t.Errorf("model = %v, want merged-model", got)
	}
	if _, ok := body["max_tokens"]; ok {
		t.Errorf("max_tokens = %v, want it removed by the null body value", body["max_tokens"])
	}
	if _, ok := body["stream"]; ok {
		t.Errorf("stream = %v, want it removed by the null body value", body["stream"])
	}
	if got := body["temperature"]; got != 0.4 {
		t.Errorf("temperature = %v, want 0.4", got)
	}
	metadata, ok := body["metadata"].(map[string]any)
	if !ok || metadata["user_id"] != "u-1" {
		t.Errorf("metadata = %#v, want the nested object", body["metadata"])
	}
}

func TestAnthropicMessagesStream(t *testing.T) {
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEvent(w, "message_start", `{"type":"message_start","message":{"id":"msg_1"}}`)
		writeEvent(w, "content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		writeEvent(w, "ping", `{"type":"ping"}`)
		// One event split across writes.
		fmt.Fprint(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"Hel")
		w.(http.Flusher).Flush()
		fmt.Fprint(w, "lo\"}}\n\n")
		w.(http.Flusher).Flush()
		writeEvent(w, "content_block_delta", `{"type":"content_block_delta","delta":{"type":"thinking_delta","thinking":"ignored"}}`)
		writeEvent(w, "content_block_delta", `{"type":"content_block_delta","delta":{"type":"signature_delta","signature":"ignored"}}`)
		writeEvent(w, "content_block_delta", `{"type":"content_block_delta","delta":{"type":"input_json_delta","partial_json":"{}"}}`)
		// Services may omit the event line; the JSON type names the event.
		writeEvent(w, "", `{"type":"content_block_delta","delta":{"type":"text_delta","text":" world"}}`)
		writeEvent(w, "content_block_stop", `{"type":"content_block_stop","index":0}`)
		writeEvent(w, "message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"}}`)
		writeEvent(w, "", `{"type":"message_stop"}`)
	})
	model := config.Model{Endpoint: srv.URL, Format: config.FormatAnthropicMessages, Model: "m"}
	text, err := streamToString(t, HTTP{Client: srv.Client()}, Request{Model: model})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if text != "Hello world" {
		t.Errorf("text = %q, want %q", text, "Hello world")
	}
}

func TestAnthropicMessagesStopsAfterDone(t *testing.T) {
	release := make(chan struct{})
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEvent(w, "content_block_delta", `{"type":"content_block_delta","delta":{"type":"text_delta","text":"done"}}`)
		writeEvent(w, "message_stop", `{"type":"message_stop"}`)
		<-release
	})
	defer close(release)
	model := config.Model{Endpoint: srv.URL, Format: config.FormatAnthropicMessages, Model: "m"}
	text, err := checkStreamPromptly(t, HTTP{Client: srv.Client()}, Request{Model: model})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if text != "done" {
		t.Errorf("text = %q, want done", text)
	}
}

func TestAnthropicMessagesStreamErrors(t *testing.T) {
	tests := []struct {
		name    string
		event   string
		typ     string
		message string
		kind    Kind
	}{
		{"overloaded", "error", "overloaded_error", "overloaded", KindLimited},
		{"rate limited", "error", "rate_limit_error", "slow down", KindLimited},
		{"too long message", "error", "api_error", "prompt is too long: 250000 tokens", KindTooLong},
		{"invalid request", "error", "invalid_request_error", "bad request", KindConfig},
		{"authentication", "error", "authentication_error", "bad key", KindConfig},
		{"permission", "error", "permission_error", "forbidden", KindConfig},
		{"not found", "error", "not_found_error", "no model", KindConfig},
		{"unknown", "error", "api_error", "boom", KindUnknown},
		{"too long wins over config", "error", "invalid_request_error", "input is too long", KindTooLong},
		{"error without event name", "", "overloaded_error", "overloaded", KindLimited},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
				writeEvent(w, tt.event, fmt.Sprintf(
					`{"type":"error","error":{"type":%q,"message":%q}}`, tt.typ, tt.message))
			})
			model := config.Model{Endpoint: srv.URL, Format: config.FormatAnthropicMessages, Model: "m"}
			_, err := streamToString(t, HTTP{Client: srv.Client()}, Request{Model: model})
			e := requireLLMError(t, err)
			if e.Kind != tt.kind {
				t.Errorf("Kind = %v, want %v (detail %q)", e.Kind, tt.kind, e.Detail)
			}
			if e.Detail != tt.message {
				t.Errorf("Detail = %q, want %q", e.Detail, tt.message)
			}
		})
	}
}

func TestAnthropicMessagesEndsWithoutDone(t *testing.T) {
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEvent(w, "content_block_delta", `{"type":"content_block_delta","delta":{"type":"text_delta","text":"tail"}}`)
	})
	model := config.Model{Endpoint: srv.URL, Format: config.FormatAnthropicMessages, Model: "m"}
	text, err := streamToString(t, HTTP{Client: srv.Client()}, Request{Model: model})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if text != "tail" {
		t.Errorf("text = %q, want tail", text)
	}
}

func TestAnthropicMessagesUnreadableEvent(t *testing.T) {
	long := strings.Repeat("x", 400)
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEvent(w, "content_block_delta", long)
	})
	model := config.Model{Endpoint: srv.URL, Format: config.FormatAnthropicMessages, Model: "m"}
	_, err := streamToString(t, HTTP{Client: srv.Client()}, Request{Model: model})
	e := requireLLMError(t, err)
	if e.Kind != KindConfig {
		t.Errorf("Kind = %v, want %v", e.Kind, KindConfig)
	}
	if want := "unreadable stream event: " + truncate(long); e.Detail != want {
		t.Errorf("Detail = %q, want %q", e.Detail, want)
	}
}
