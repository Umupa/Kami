package llm

import (
	"encoding/json"
	"net/http"
)

// Preview returns the headers and body Stream would send for req, with the
// key masked, for diagnostics.
func Preview(req Request) (http.Header, []byte, error) {
	f, ok := formats[req.Model.Format]
	if !ok {
		return nil, nil, &Error{Kind: KindConfig, Detail: "unsupported format " + string(req.Model.Format)}
	}
	body := f.body(req)
	mergeBody(body, req.Model.Body)
	data, err := json.MarshalIndent(body, "", "  ")
	if err != nil {
		return nil, nil, err
	}
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Accept", "text/event-stream")
	f.headers(h)
	setAuth(h, req.Model.Auth, "••••")
	for k, v := range req.Model.Headers {
		h.Set(k, mask(v, req.Key))
	}
	return h, data, nil
}
