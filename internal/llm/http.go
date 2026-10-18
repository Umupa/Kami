package llm

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/Umupa/cida-next/internal/config"
)

// Timeouts of a request. A reply may take long to finish, but never goes
// quiet for long: reasoning models send keep-alives while they think.
const (
	connectTimeout = 15 * time.Second
	headerTimeout  = 90 * time.Second
	idleTimeout    = 90 * time.Second
)

// format is one wire format: the request body it sends and the stream events
// it reads.
type format interface {
	// body returns the request body before the model's extra body is merged
	// into it.
	body(req Request) map[string]any
	// headers adds the headers the format needs, before the model's own.
	headers(h http.Header)
	// event reads one server-sent event, calling onText with the text it
	// carries. done reports the end of the reply.
	event(name, data string, onText func(string)) (done bool, err error)
}

var formats = map[config.Format]format{
	config.FormatChatCompletions: chatCompletions{},
}

// HTTP streams completions over HTTP in every format.
type HTTP struct {
	// Client sends the requests; nil uses one with the timeouts above.
	Client *http.Client
}

var defaultClient = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	DialContext:           (&net.Dialer{Timeout: connectTimeout}).DialContext,
	TLSHandshakeTimeout:   connectTimeout,
	ResponseHeaderTimeout: headerTimeout,
	ForceAttemptHTTP2:     true,
}}

func (h HTTP) Stream(ctx context.Context, req Request, onText func(string)) error {
	f, ok := formats[req.Model.Format]
	if !ok {
		return &Error{Kind: KindConfig, Detail: fmt.Sprintf("unsupported format %q", req.Model.Format)}
	}
	body := f.body(req)
	mergeBody(body, req.Model.Body)
	data, err := json.Marshal(body)
	if err != nil {
		return &Error{Kind: KindConfig, Detail: "request body: " + err.Error(), Err: err}
	}

	// The stream is cancelled when it goes quiet for idleTimeout.
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	idle := time.AfterFunc(idleTimeout, func() { cancel(errIdle) })
	defer idle.Stop()

	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, req.Model.Endpoint, bytes.NewReader(data))
	if err != nil {
		return &Error{Kind: KindConfig, Detail: "endpoint: " + err.Error(), Err: err}
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Accept", "text/event-stream")
	f.headers(hr.Header)
	setAuth(hr.Header, req.Model.Auth, req.Key)
	for k, v := range req.Model.Headers {
		hr.Header.Set(k, v)
	}

	client := h.Client
	if client == nil {
		client = defaultClient
	}
	resp, err := client.Do(hr)
	if err != nil {
		return transportError(ctx, err, req.Key)
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
		return statusError(resp.StatusCode, mask(string(detail), req.Key))
	}

	done := false
	err = readEvents(resp.Body, func(name, data string) error {
		idle.Reset(idleTimeout)
		d, err := f.event(name, data, onText)
		done = done || d
		if d {
			return errStop
		}
		return err
	})
	switch {
	case errors.Is(err, errStop) || err == nil && done:
		return nil
	case err == nil:
		// Some services end the stream without a final event.
		return nil
	case ctx.Err() != nil:
		return transportError(ctx, err, req.Key)
	}
	var le *Error
	if errors.As(err, &le) {
		le.Detail = mask(le.Detail, req.Key)
		return le
	}
	return &Error{Kind: KindUnknown, Detail: mask(err.Error(), req.Key), Err: err}
}

var (
	errIdle = errors.New("the stream went quiet")
	errStop = errors.New("stop reading")
)

// setAuth puts the key in the header auth names.
func setAuth(h http.Header, auth config.Auth, key string) {
	if key == "" {
		return
	}
	switch auth {
	case config.AuthBearer:
		h.Set("Authorization", "Bearer "+key)
	case config.AuthXAPIKey:
		h.Set("X-Api-Key", key)
	case config.AuthAPIKey:
		h.Set("Api-Key", key)
	}
}

// mergeBody merges extra into body level by level; a null value removes the
// key, so that the configuration can drop what the app sends.
func mergeBody(body, extra map[string]any) {
	for k, v := range extra {
		if v == nil {
			delete(body, k)
			continue
		}
		sub, isMap := v.(map[string]any)
		cur, curIsMap := body[k].(map[string]any)
		if isMap && curIsMap {
			mergeBody(cur, sub)
			continue
		}
		body[k] = v
	}
}

// mask hides the key wherever a service echoes it back.
func mask(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "••••")
}

// readEvents reads server-sent events from r and calls fn with each, until
// r ends or fn returns an error.
func readEvents(r io.Reader, fn func(name, data string) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 16<<20)
	var name string
	var data []string
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if len(data) > 0 {
				if err := fn(name, strings.Join(data, "\n")); err != nil {
					return err
				}
			}
			name, data = "", data[:0]
		case strings.HasPrefix(line, ":"):
			// A comment, sent as a keep-alive.
			if err := fn("", ""); err != nil && !errors.Is(err, errStop) {
				return err
			}
		default:
			field, value, _ := strings.Cut(line, ":")
			value = strings.TrimPrefix(value, " ")
			switch field {
			case "event":
				name = value
			case "data":
				data = append(data, value)
			}
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	if len(data) > 0 {
		return fn(name, strings.Join(data, "\n"))
	}
	return nil
}

// transportError classifies a failure to reach the service or to read its
// reply.
func transportError(ctx context.Context, err error, key string) error {
	if errors.Is(context.Cause(ctx), errIdle) {
		return &Error{Kind: KindTimeout, Detail: errIdle.Error(), Err: err}
	}
	if ctx.Err() != nil {
		return context.Canceled
	}
	detail := mask(err.Error(), key)
	var (
		dnsErr  *net.DNSError
		opErr   *net.OpError
		netErr  net.Error
		certErr *tls.CertificateVerificationError
		unkAuth x509.UnknownAuthorityError
		hostErr x509.HostnameError
		recErr  tls.RecordHeaderError
		alert   tls.AlertError
	)
	switch {
	case errors.As(err, &certErr), errors.As(err, &unkAuth), errors.As(err, &hostErr),
		errors.As(err, &recErr), errors.As(err, &alert):
		return &Error{Kind: KindTLS, Detail: detail, Err: err}
	case errors.As(err, &netErr) && netErr.Timeout():
		return &Error{Kind: KindTimeout, Detail: detail, Err: err}
	case errors.As(err, &dnsErr), errors.As(err, &opErr):
		return &Error{Kind: KindOffline, Detail: detail, Err: err}
	}
	return &Error{Kind: KindUnknown, Detail: detail, Err: err}
}

// statusError classifies an HTTP error response.
func statusError(status int, detail string) *Error {
	e := &Error{Kind: KindUnknown, Status: status, Detail: detail}
	switch {
	case status == http.StatusTooManyRequests:
		e.Kind = KindLimited
	case status == http.StatusRequestTimeout || status == http.StatusGatewayTimeout:
		e.Kind = KindTimeout
	case status == http.StatusRequestEntityTooLarge || tooLong(detail):
		e.Kind = KindTooLong
	case status == http.StatusUnauthorized, status == http.StatusForbidden,
		status == http.StatusNotFound, status == http.StatusBadRequest,
		status == http.StatusUnprocessableEntity:
		e.Kind = KindConfig
	}
	return e
}

// tooLong reports whether a service's error says the request is longer than
// the model takes.
func tooLong(detail string) bool {
	d := strings.ToLower(detail)
	for _, s := range []string{"context_length_exceeded", "context length", "maximum context", "too many tokens", "prompt is too long", "input is too long", "max_tokens"} {
		if strings.Contains(d, s) {
			return true
		}
	}
	return false
}
