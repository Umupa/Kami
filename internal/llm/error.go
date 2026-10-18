package llm

import "fmt"

// Kind is a class of failure, by what the user can do about it.
type Kind int

const (
	KindUnknown Kind = iota
	// KindTimeout: the service did not answer in time; retry.
	KindTimeout
	// KindOffline: no connection or no name resolution; check the network.
	KindOffline
	// KindLimited: rate limits or quota; retry later.
	KindLimited
	// KindTLS: the secure connection failed; check the network or proxy.
	KindTLS
	// KindConfig: the service rejected the request as configured, or its
	// reply could not be read; the configuration needs fixing.
	KindConfig
	// KindTooLong: the request exceeds what the model takes.
	KindTooLong
)

// Error is a failed request. Detail is the service's own message, with the
// API key masked, for diagnostics; it never reaches the panel.
type Error struct {
	Kind   Kind
	Status int // the HTTP status, 0 without a response
	Detail string
	Err    error
}

func (e *Error) Error() string {
	if e.Status != 0 {
		return fmt.Sprintf("llm: %s (HTTP %d): %s", e.Kind, e.Status, e.Detail)
	}
	return fmt.Sprintf("llm: %s: %s", e.Kind, e.Detail)
}

func (e *Error) Unwrap() error { return e.Err }

func (k Kind) String() string {
	switch k {
	case KindTimeout:
		return "timeout"
	case KindOffline:
		return "offline"
	case KindLimited:
		return "limited"
	case KindTLS:
		return "tls"
	case KindConfig:
		return "config"
	case KindTooLong:
		return "too long"
	}
	return "unknown"
}
