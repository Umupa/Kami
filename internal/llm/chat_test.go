package llm

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Umupa/Gloss/internal/config"
)

func TestChatCompletionsRequest(t *testing.T) {
	t.Run("with system", func(t *testing.T) {
		srv, recCh := captureServer(t)
		model := config.Model{
			Name:     "test",
			Endpoint: srv.URL + "/v1/chat/completions",
			Format:   config.FormatChatCompletions,
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
		checkRequestBasics(t, rec, "/v1/chat/completions")
		if got := rec.header.Get("X-Trace"); got != "trace-1" {
			t.Errorf("X-Trace = %q, want trace-1", got)
		}
		if got := rec.header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("Authorization = %q, want Bearer secret", got)
		}
		want := map[string]any{
			"model":  "gpt-test",
			"stream": true,
			"messages": []any{
				map[string]any{"role": "system", "content": "be brief"},
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
			Endpoint: srv.URL + "/chat",
			Format:   config.FormatChatCompletions,
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
		checkRequestBasics(t, rec, "/chat")
		want := map[string]any{
			"model":  "gpt-test",
			"stream": true,
			"messages": []any{
				map[string]any{"role": "user", "content": "hi"},
			},
		}
		if body := decodeRequestBody(t, rec); !reflect.DeepEqual(body, want) {
			t.Errorf("body = %#v, want %#v", body, want)
		}
	})
}

func TestChatCompletionsAuth(t *testing.T) {
	testAuthHeaders(t, config.FormatChatCompletions)
}

func TestChatCompletionsBodyMerge(t *testing.T) {
	srv, recCh := captureServer(t)
	model := config.Model{
		Endpoint: srv.URL + "/chat",
		Format:   config.FormatChatCompletions,
		Model:    "gpt-test",
		Body: map[string]any{
			"model":       "merged-model",
			"stream":      nil,
			"metadata":    map[string]any{"user_id": "u-1"},
			"temperature": 0.4,
		},
	}
	err := (HTTP{Client: srv.Client()}).Stream(context.Background(),
		Request{Model: model, Messages: []Message{{Role: RoleUser, Content: "hi"}}}, func(string) {})
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
	if got := body["temperature"]; got != 0.4 {
		t.Errorf("temperature = %v, want 0.4", got)
	}
	metadata, ok := body["metadata"].(map[string]any)
	if !ok || metadata["user_id"] != "u-1" {
		t.Errorf("metadata = %#v, want the nested object", body["metadata"])
	}
}

func TestChatCompletionsStream(t *testing.T) {
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		// One event split across writes.
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"co")
		w.(http.Flusher).Flush()
		fmt.Fprint(w, "ntent\":\"Hello\"}}]}\n\n")
		w.(http.Flusher).Flush()
		// A comment may arrive between events and is a keep-alive.
		fmt.Fprint(w, ": keep-alive\n")
		w.(http.Flusher).Flush()
		writeEvent(w, "", `{"choices":[{"delta":{"content":" world"}}]}`)
		writeEvent(w, "", "[DONE]")
	})
	model := config.Model{Endpoint: srv.URL, Format: config.FormatChatCompletions, Model: "m"}
	text, err := streamToString(t, HTTP{Client: srv.Client()}, Request{Model: model})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if text != "Hello world" {
		t.Errorf("text = %q, want %q", text, "Hello world")
	}
}

func TestChatCompletionsStopsAfterDone(t *testing.T) {
	release := make(chan struct{})
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEvent(w, "", `{"choices":[{"delta":{"content":"done"}}]}`)
		writeEvent(w, "", "[DONE]")
		<-release
	})
	defer close(release)
	model := config.Model{Endpoint: srv.URL, Format: config.FormatChatCompletions, Model: "m"}
	text, err := checkStreamPromptly(t, HTTP{Client: srv.Client()}, Request{Model: model})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if text != "done" {
		t.Errorf("text = %q, want done", text)
	}
}

func TestChatCompletionsStreamErrors(t *testing.T) {
	tests := []struct {
		name    string
		message string
		code    string
		kind    Kind
	}{
		{"too long", "prompt is too long", "", KindTooLong},
		{"unknown", "boom", "server_error", KindUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
				writeEvent(w, "", fmt.Sprintf(`{"error":{"message":%q,"code":%q}}`, tt.message, tt.code))
			})
			model := config.Model{Endpoint: srv.URL, Format: config.FormatChatCompletions, Model: "m"}
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

func TestChatCompletionsEndsWithoutDone(t *testing.T) {
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEvent(w, "", `{"choices":[{"delta":{"content":"tail"}}]}`)
	})
	model := config.Model{Endpoint: srv.URL, Format: config.FormatChatCompletions, Model: "m"}
	text, err := streamToString(t, HTTP{Client: srv.Client()}, Request{Model: model})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	if text != "tail" {
		t.Errorf("text = %q, want tail", text)
	}
}

func TestChatCompletionsUnreadableEvent(t *testing.T) {
	long := strings.Repeat("x", 400)
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEvent(w, "", long)
	})
	model := config.Model{Endpoint: srv.URL, Format: config.FormatChatCompletions, Model: "m"}
	_, err := streamToString(t, HTTP{Client: srv.Client()}, Request{Model: model})
	e := requireLLMError(t, err)
	if e.Kind != KindConfig {
		t.Errorf("Kind = %v, want %v", e.Kind, KindConfig)
	}
	if want := "unreadable stream event: " + truncate(long); e.Detail != want {
		t.Errorf("Detail = %q, want %q", e.Detail, want)
	}
}
