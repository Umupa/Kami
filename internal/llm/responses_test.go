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

func TestResponsesRequest(t *testing.T) {
	t.Run("with system", func(t *testing.T) {
		srv, recCh := captureServer(t)
		model := config.Model{
			Endpoint: srv.URL + "/v1/responses",
			Format:   config.FormatResponses,
			Model:    "gpt-test",
			Auth:     config.AuthBearer,
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
		checkRequestBasics(t, rec, "/v1/responses")
		if got := rec.header.Get("X-Trace"); got != "trace-1" {
			t.Errorf("X-Trace = %q, want trace-1", got)
		}
		if got := rec.header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q, want Bearer secret", got)
		}
		want := map[string]any{
			"model":        "gpt-test",
			"stream":       true,
			"store":        false,
			"instructions": "be brief",
			"input": []any{
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
			Endpoint: srv.URL + "/v1/responses",
			Format:   config.FormatResponses,
			Model:    "gpt-test",
		}
		err := (HTTP{Client: srv.Client()}).Stream(context.Background(), Request{
			Model:    model,
			Messages: []Message{{Role: RoleUser, Content: "hi"}},
		}, func(string) {})
		if err != nil {
			t.Fatalf("Stream: %v", err)
		}
		rec := <-recCh
		checkRequestBasics(t, rec, "/v1/responses")
		body := decodeRequestBody(t, rec)
		if _, ok := body["instructions"]; ok {
			t.Errorf("instructions = %v, want it omitted", body["instructions"])
		}
		want := map[string]any{
			"model":  "gpt-test",
			"stream": true,
			"store":  false,
			"input": []any{
				map[string]any{"role": "user", "content": "hi"},
			},
		}
		if !reflect.DeepEqual(body, want) {
			t.Errorf("body = %#v, want %#v", body, want)
		}
	})
}

func TestResponsesAuth(t *testing.T) {
	testAuthHeaders(t, config.FormatResponses)
}

func TestResponsesBodyMerge(t *testing.T) {
	srv, recCh := captureServer(t)
	model := config.Model{
		Endpoint: srv.URL + "/v1/responses",
		Format:   config.FormatResponses,
		Model:    "gpt-test",
		Body: map[string]any{
			"model":        "merged-model",
			"stream":       nil,
			"store":        true,
			"instructions": nil,
			"metadata":     map[string]any{"user_id": "u-1"},
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
	if _, ok := body["stream"]; ok {
		t.Errorf("stream = %v, want it removed by the null body value", body["stream"])
	}
	if _, ok := body["instructions"]; ok {
		t.Errorf("instructions = %v, want it removed by the null body value", body["instructions"])
	}
	if got := body["store"]; got != true {
		t.Errorf("store = %v, want true", got)
	}
	metadata, ok := body["metadata"].(map[string]any)
	if !ok || metadata["user_id"] != "u-1" {
		t.Errorf("metadata = %#v, want the nested object", body["metadata"])
	}
}

func TestResponsesStream(t *testing.T) {
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEvent(w, "response.created", `{"type":"response.created","response":{}}`)
		writeEvent(w, "response.in_progress", `{"type":"response.in_progress","response":{}}`)
		writeEvent(w, "response.output_item.added", `{"type":"response.output_item.added","output_index":0}`)
		writeEvent(w, "response.reasoning_summary_text.delta", `{"type":"response.reasoning_summary_text.delta","delta":"ignored thought"}`)
		// One event split across writes.
		fmt.Fprint(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"Hel")
		w.(http.Flusher).Flush()
		fmt.Fprint(w, "lo\"}\n\n")
		w.(http.Flusher).Flush()
		// Services may omit the event line; the JSON type names the event.
		writeEvent(w, "", `{"type":"response.output_text.delta","delta":" world"}`)
		writeEvent(w, "response.completed", `{"type":"response.completed","response":{}}`)
	})
	model := config.Model{Endpoint: srv.URL, Format: config.FormatResponses, Model: "m"}
	text, err := streamToString(t, HTTP{Client: srv.Client()}, Request{Model: model})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if text != "Hello world" {
		t.Errorf("text = %q, want %q", text, "Hello world")
	}
}

func TestResponsesIncomplete(t *testing.T) {
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEvent(w, "response.output_text.delta", `{"type":"response.output_text.delta","delta":"cut"}`)
		writeEvent(w, "response.incomplete", `{"type":"response.incomplete","response":{"incomplete_details":{"reason":"max_output_tokens"}}}`)
	})
	model := config.Model{Endpoint: srv.URL, Format: config.FormatResponses, Model: "m"}
	text, err := streamToString(t, HTTP{Client: srv.Client()}, Request{Model: model})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if text != "cut" {
		t.Errorf("text = %q, want cut", text)
	}
}

func TestResponsesStopsAfterDone(t *testing.T) {
	release := make(chan struct{})
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEvent(w, "response.output_text.delta", `{"type":"response.output_text.delta","delta":"done"}`)
		writeEvent(w, "response.completed", `{"type":"response.completed","response":{}}`)
		<-release
	})
	defer close(release)
	model := config.Model{Endpoint: srv.URL, Format: config.FormatResponses, Model: "m"}
	text, err := checkStreamPromptly(t, HTTP{Client: srv.Client()}, Request{Model: model})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if text != "done" {
		t.Errorf("text = %q, want done", text)
	}
}

func TestResponsesStreamErrors(t *testing.T) {
	tests := []struct {
		name  string
		event string
		data  string
		kind  Kind
		text  string
	}{
		{
			name:  "failed context length code",
			event: "response.failed",
			data:  `{"type":"response.failed","response":{"error":{"code":"context_length_exceeded","message":"too many tokens"}}}`,
			kind:  KindTooLong,
			text:  "too many tokens",
		},
		{
			name:  "failed long message",
			event: "response.failed",
			data:  `{"type":"response.failed","response":{"error":{"code":"server_error","message":"maximum context length is 1000 tokens"}}}`,
			kind:  KindTooLong,
			text:  "maximum context length is 1000 tokens",
		},
		{
			name:  "failed unknown",
			event: "response.failed",
			data:  `{"type":"response.failed","response":{"error":{"code":"server_error","message":"boom"}}}`,
			kind:  KindUnknown,
			text:  "boom",
		},
		{
			name:  "error context length code without event name",
			event: "",
			data:  `{"type":"error","code":"context_length_exceeded","message":"too long"}`,
			kind:  KindTooLong,
			text:  "too long",
		},
		{
			name:  "error long message",
			event: "error",
			data:  `{"type":"error","code":"invalid_request_error","message":"input is too long"}`,
			kind:  KindTooLong,
			text:  "input is too long",
		},
		{
			name:  "error unknown",
			event: "error",
			data:  `{"type":"error","code":"server_error","message":"boom"}`,
			kind:  KindUnknown,
			text:  "boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
				writeEvent(w, tt.event, tt.data)
			})
			model := config.Model{Endpoint: srv.URL, Format: config.FormatResponses, Model: "m"}
			_, err := streamToString(t, HTTP{Client: srv.Client()}, Request{Model: model})
			e := requireLLMError(t, err)
			if e.Kind != tt.kind {
				t.Errorf("Kind = %v, want %v (detail %q)", e.Kind, tt.kind, e.Detail)
			}
			if e.Detail != tt.text {
				t.Errorf("Detail = %q, want %q", e.Detail, tt.text)
			}
		})
	}
}

func TestResponsesEndsWithoutDone(t *testing.T) {
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEvent(w, "response.output_text.delta", `{"type":"response.output_text.delta","delta":"tail"}`)
	})
	model := config.Model{Endpoint: srv.URL, Format: config.FormatResponses, Model: "m"}
	text, err := streamToString(t, HTTP{Client: srv.Client()}, Request{Model: model})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if text != "tail" {
		t.Errorf("text = %q, want tail", text)
	}
}

func TestResponsesUnreadableEvent(t *testing.T) {
	long := strings.Repeat("x", 400)
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEvent(w, "response.output_text.delta", long)
	})
	model := config.Model{Endpoint: srv.URL, Format: config.FormatResponses, Model: "m"}
	_, err := streamToString(t, HTTP{Client: srv.Client()}, Request{Model: model})
	e := requireLLMError(t, err)
	if e.Kind != KindConfig {
		t.Errorf("Kind = %v, want %v", e.Kind, KindConfig)
	}
	if want := "unreadable stream event: " + truncate(long); e.Detail != want {
		t.Errorf("Detail = %q, want %q", e.Detail, want)
	}
}
