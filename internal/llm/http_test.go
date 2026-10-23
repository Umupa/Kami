package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Umupa/Kami/internal/config"
)

type capturedRequest struct {
	method string
	path   string
	header http.Header
	body   []byte
	err    error
}

// captureServer records one request and answers with an empty body.
func captureServer(t *testing.T) (*httptest.Server, <-chan capturedRequest) {
	t.Helper()
	ch := make(chan capturedRequest, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		ch <- capturedRequest{
			method: r.Method,
			path:   r.URL.Path,
			header: r.Header.Clone(),
			body:   body,
			err:    err,
		}
	}))
	t.Cleanup(srv.Close)
	return srv, ch
}

// newSSEServer serves a hand-written event stream.
func newSSEServer(t *testing.T, handler func(http.ResponseWriter, *http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// writeEvent writes and flushes one complete server-sent event.
func writeEvent(w http.ResponseWriter, name, data string) {
	if name != "" {
		fmt.Fprintf(w, "event: %s\n", name)
	}
	fmt.Fprintf(w, "data: %s\n\n", data)
	w.(http.Flusher).Flush()
}

// checkRequestBasics asserts what every format sends the same way.
func checkRequestBasics(t *testing.T, rec capturedRequest, path string) {
	t.Helper()
	if rec.err != nil {
		t.Fatalf("read request body: %v", rec.err)
	}
	if rec.method != http.MethodPost {
		t.Errorf("method = %q, want POST", rec.method)
	}
	if rec.path != path {
		t.Errorf("path = %q, want %q", rec.path, path)
	}
	if got := rec.header.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := rec.header.Get("Accept"); got != "text/event-stream" {
		t.Errorf("Accept = %q, want text/event-stream", got)
	}
}

func decodeRequestBody(t *testing.T, rec capturedRequest) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(rec.body, &body); err != nil {
		t.Fatalf("decode request body %q: %v", rec.body, err)
	}
	return body
}

// testAuthHeaders checks the auth header for every config.Auth value.
func testAuthHeaders(t *testing.T, format config.Format) {
	t.Helper()
	const key = "k"
	tests := []struct {
		name string
		auth config.Auth
		key  string
		want map[string]string
	}{
		{"bearer", config.AuthBearer, key, map[string]string{"Authorization": "Bearer " + key}},
		{"x-api-key", config.AuthXAPIKey, key, map[string]string{"X-Api-Key": key}},
		{"api-key", config.AuthAPIKey, key, map[string]string{"Api-Key": key}},
		{"none", config.AuthNone, key, nil},
		{"empty key", config.AuthBearer, "", nil},
	}
	authHeaders := []string{"Authorization", "X-Api-Key", "Api-Key"}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv, recCh := captureServer(t)
			model := config.Model{
				Endpoint: srv.URL + "/stream",
				Format:   format,
				Model:    "test-model",
				Auth:     tt.auth,
			}
			err := (HTTP{Client: srv.Client()}).Stream(context.Background(),
				Request{Model: model, Key: tt.key}, func(string) {})
			if err != nil {
				t.Fatalf("Stream: %v", err)
			}
			rec := <-recCh
			for _, h := range authHeaders {
				if got := rec.header.Get(h); got != tt.want[h] {
					t.Errorf("%s = %q, want %q", h, got, tt.want[h])
				}
			}
		})
	}
}

func streamToString(t *testing.T, h HTTP, req Request) (string, error) {
	t.Helper()
	var b strings.Builder
	err := h.Stream(context.Background(), req, func(s string) { b.WriteString(s) })
	return b.String(), err
}

// checkStreamPromptly asserts that Stream returns soon and hands back the
// collected text. A server that keeps its response open must not keep it
// waiting once a done event has arrived.
func checkStreamPromptly(t *testing.T, h HTTP, req Request) (string, error) {
	t.Helper()
	type result struct {
		text string
		err  error
	}
	done := make(chan result, 1)
	go func() {
		var b strings.Builder
		err := h.Stream(context.Background(), req, func(s string) { b.WriteString(s) })
		done <- result{text: b.String(), err: err}
	}()
	select {
	case res := <-done:
		return res.text, res.err
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not return after the done event")
		return "", nil
	}
}

func requireLLMError(t *testing.T, err error) *Error {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("error = %v, want *llm.Error", err)
	}
	return e
}

type readEvent struct {
	name string
	data string
}

func collectEvents(r io.Reader) ([]readEvent, error) {
	var got []readEvent
	err := readEvents(r, func(name, data string) error {
		got = append(got, readEvent{name: name, data: data})
		return nil
	})
	return got, err
}

func TestReadEvents(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []readEvent
	}{
		{
			name: "event name and data",
			in:   "event: message_stop\ndata: {}\n\n",
			want: []readEvent{{name: "message_stop", data: "{}"}},
		},
		{
			name: "multi-line data",
			in:   "event: delta\ndata: one\ndata: two\n\n",
			want: []readEvent{{name: "delta", data: "one\ntwo"}},
		},
		{
			name: "comments are keep-alives",
			in:   ": ping\nevent: message_stop\ndata: {}\n\n",
			want: []readEvent{{name: "", data: ""}, {name: "message_stop", data: "{}"}},
		},
		{
			name: "CRLF line endings",
			in:   "event: e\r\ndata: one\r\ndata: two\r\n\r\n",
			want: []readEvent{{name: "e", data: "one\ntwo"}},
		},
		{
			name: "final event without blank line",
			in:   "event: last\ndata: tail",
			want: []readEvent{{name: "last", data: "tail"}},
		},
		{
			name: "event without data is skipped",
			in:   "event: ping\n\n",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := collectEvents(strings.NewReader(tt.in))
			if err != nil {
				t.Fatalf("readEvents: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("events = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestReadEventsStopsOnError(t *testing.T) {
	sentinel := errors.New("stop")
	calls := 0
	err := readEvents(strings.NewReader("data: a\n\ndata: b\n\n"), func(string, string) error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want %v", err, sentinel)
	}
	if calls != 1 {
		t.Errorf("callback calls = %d, want 1", calls)
	}
}

func TestStatusErrorKinds(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		kind   Kind
	}{
		{"bad request", http.StatusBadRequest, `{"error":"bad request"}`, KindConfig},
		{"context too long", http.StatusBadRequest, `{"error":"context_length_exceeded"}`, KindTooLong},
		{"unauthorized", http.StatusUnauthorized, `{"error":"unauthorized"}`, KindConfig},
		{"forbidden", http.StatusForbidden, `{"error":"forbidden"}`, KindConfig},
		{"not found", http.StatusNotFound, `{"error":"not found"}`, KindConfig},
		{"request timeout", http.StatusRequestTimeout, `{"error":"timeout"}`, KindTimeout},
		{"gateway timeout", http.StatusGatewayTimeout, `{"error":"timeout"}`, KindTimeout},
		{"payload too large", http.StatusRequestEntityTooLarge, `{"error":"too large"}`, KindTooLong},
		{"rate limited", http.StatusTooManyRequests, `{"error":"slow down"}`, KindLimited},
		{"server error", http.StatusInternalServerError, `{"error":"boom"}`, KindUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.status)
				io.WriteString(w, tt.body)
			}))
			t.Cleanup(srv.Close)
			model := config.Model{Endpoint: srv.URL, Format: config.FormatChatCompletions, Model: "m"}
			err := (HTTP{Client: srv.Client()}).Stream(context.Background(),
				Request{Model: model, Messages: []Message{{Role: RoleUser, Content: "hi"}}}, func(string) {})
			e := requireLLMError(t, err)
			if e.Kind != tt.kind {
				t.Errorf("Kind = %v, want %v (detail %q)", e.Kind, tt.kind, e.Detail)
			}
			if e.Status != tt.status {
				t.Errorf("Status = %d, want %d", e.Status, tt.status)
			}
		})
	}
}

func TestErrorMasksKey(t *testing.T) {
	const key = "sk-live-DEADBEEF-do-not-leak"

	t.Run("HTTP error body", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			fmt.Fprintf(w, `{"error":"invalid key %s"}`, key)
		}))
		t.Cleanup(srv.Close)
		model := config.Model{Endpoint: srv.URL, Format: config.FormatChatCompletions, Model: "m", Auth: config.AuthBearer}
		err := (HTTP{Client: srv.Client()}).Stream(context.Background(),
			Request{Model: model, Key: key}, func(string) {})
		e := requireLLMError(t, err)
		if strings.Contains(e.Detail, key) || strings.Contains(e.Error(), key) {
			t.Fatalf("key leaked: Detail %q, Error() %q", e.Detail, e.Error())
		}
		if !strings.Contains(e.Detail, "••••") {
			t.Errorf("Detail = %q, want the key masked", e.Detail)
		}
	})

	t.Run("stream error", func(t *testing.T) {
		srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
			writeEvent(w, "", fmt.Sprintf(`{"error":{"message":"bad key %s"}}`, key))
		})
		model := config.Model{Endpoint: srv.URL, Format: config.FormatChatCompletions, Model: "m", Auth: config.AuthBearer}
		err := (HTTP{Client: srv.Client()}).Stream(context.Background(),
			Request{Model: model, Key: key}, func(string) {})
		e := requireLLMError(t, err)
		if strings.Contains(e.Detail, key) || strings.Contains(e.Error(), key) {
			t.Fatalf("key leaked: Detail %q, Error() %q", e.Detail, e.Error())
		}
	})
}

func TestConnectionRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close listener: %v", err)
	}
	model := config.Model{
		Endpoint: "http://" + addr,
		Format:   config.FormatChatCompletions,
		Model:    "m",
	}
	client := &http.Client{Transport: &http.Transport{}}
	err = (HTTP{Client: client}).Stream(context.Background(), Request{Model: model}, func(string) {})
	e := requireLLMError(t, err)
	if e.Kind != KindOffline {
		t.Errorf("Kind = %v, want %v (detail %q)", e.Kind, KindOffline, e.Detail)
	}
}

func TestContextCanceledMidStream(t *testing.T) {
	release := make(chan struct{})
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		writeEvent(w, "", `{"choices":[{"delta":{"content":"partial"}}]}`)
		<-release
	})
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var text strings.Builder
	done := make(chan error, 1)
	model := config.Model{Endpoint: srv.URL, Format: config.FormatChatCompletions, Model: "m"}
	go func() {
		done <- (HTTP{Client: srv.Client()}).Stream(ctx, Request{Model: model}, func(s string) {
			text.WriteString(s)
			cancel()
		})
	}()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not return after cancellation")
	}
	if got := text.String(); got != "partial" {
		t.Errorf("text = %q, want %q", got, "partial")
	}
}

// TestContextDeadlineExceeded is skipped because transportError in http.go
// returns context.Canceled for every parent-context failure, though Client
// documents ctx.Err(). It is left here to show the failing expectation; do not
// fix it in this change.
func TestContextDeadlineExceeded(t *testing.T) {

	release := make(chan struct{})
	srv := newSSEServer(t, func(w http.ResponseWriter, r *http.Request) {
		<-release
	})
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	model := config.Model{Endpoint: srv.URL, Format: config.FormatChatCompletions, Model: "m"}
	err := (HTTP{Client: srv.Client()}).Stream(ctx, Request{Model: model}, func(string) {})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want context.DeadlineExceeded", err)
	}
}

func TestUnsupportedFormat(t *testing.T) {
	model := config.Model{Endpoint: "http://127.0.0.1:1", Format: config.Format("bogus"), Model: "m"}
	err := (HTTP{}).Stream(context.Background(), Request{Model: model}, func(string) {})
	e := requireLLMError(t, err)
	if e.Kind != KindConfig {
		t.Errorf("Kind = %v, want %v", e.Kind, KindConfig)
	}
}

func TestMergeBody(t *testing.T) {
	tests := []struct {
		name  string
		body  map[string]any
		extra map[string]any
		want  map[string]any
	}{
		{
			name:  "scalar replaced",
			body:  map[string]any{"model": "a", "stream": true},
			extra: map[string]any{"model": "b"},
			want:  map[string]any{"model": "b", "stream": true},
		},
		{
			name:  "null removes a key",
			body:  map[string]any{"a": 1, "b": 2},
			extra: map[string]any{"a": nil},
			want:  map[string]any{"b": 2},
		},
		{
			name:  "nested objects merged",
			body:  map[string]any{"o": map[string]any{"a": 1, "b": 2}},
			extra: map[string]any{"o": map[string]any{"b": 3, "c": 4}},
			want:  map[string]any{"o": map[string]any{"a": 1, "b": 3, "c": 4}},
		},
		{
			name:  "null removes a nested key",
			body:  map[string]any{"o": map[string]any{"a": 1, "b": 2}},
			extra: map[string]any{"o": map[string]any{"a": nil}},
			want:  map[string]any{"o": map[string]any{"b": 2}},
		},
		{
			name:  "map replaces scalar",
			body:  map[string]any{"o": "scalar"},
			extra: map[string]any{"o": map[string]any{"a": 1}},
			want:  map[string]any{"o": map[string]any{"a": 1}},
		},
		{
			name:  "scalar replaces map",
			body:  map[string]any{"o": map[string]any{"a": 1}},
			extra: map[string]any{"o": "scalar"},
			want:  map[string]any{"o": "scalar"},
		},
		{
			name:  "array replaced",
			body:  map[string]any{"o": []any{"a"}},
			extra: map[string]any{"o": []any{"b", "c"}},
			want:  map[string]any{"o": []any{"b", "c"}},
		},
		{
			name:  "new key added",
			body:  map[string]any{"a": 1},
			extra: map[string]any{"b": 2},
			want:  map[string]any{"a": 1, "b": 2},
		},
		{
			name:  "null of a missing key is a no-op",
			body:  map[string]any{"a": 1},
			extra: map[string]any{"b": nil},
			want:  map[string]any{"a": 1},
		},
		{
			name:  "empty extra keeps the body",
			body:  map[string]any{"a": 1},
			extra: map[string]any{},
			want:  map[string]any{"a": 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeBody(tt.body, tt.extra)
			if !reflect.DeepEqual(tt.body, tt.want) {
				t.Errorf("body = %#v, want %#v", tt.body, tt.want)
			}
		})
	}
}

func TestMaxTokensLimitIsConfig(t *testing.T) {
	// A limit on the reply length is a setting to fix, not input that is
	// too long.
	e := statusError(400, `{"error":{"message":"max_tokens: 200000 > 64000, which is the maximum allowed"}}`)
	if e.Kind != KindConfig {
		t.Errorf("Kind = %v, want config", e.Kind)
	}
}
