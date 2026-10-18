// Package keyring keeps the API key of each model configuration in the login
// keychain. Keys never appear in config.json, logs or command output.
package keyring

import (
	"errors"
	"sync"

	"github.com/egoist/mygo"
)

// ErrNotFound is what Get returns when no key is stored for a model.
var ErrNotFound = errors.New("keyring: no key stored")

// Keyring stores one API key per model name.
type Keyring interface {
	Get(model string) (string, error)
	Set(model, key string) error
	Delete(model string) error
}

// System returns the login keychain, under the app's identifier. The app and
// its command line are the same signed executable, so neither prompts to read
// what the other stored.
func System() Keyring { return system{} }

type system struct{}

func account(model string) string { return "model/" + model }

func (system) Get(model string) (string, error) {
	b, err := mygo.Secrets.Get(account(model))
	if errors.Is(err, mygo.ErrSecretNotFound) {
		return "", ErrNotFound
	}
	return string(b), err
}

func (system) Set(model, key string) error { return mygo.Secrets.Set(account(model), []byte(key)) }

func (system) Delete(model string) error { return mygo.Secrets.Delete(account(model)) }

// Memory is a Keyring in memory, for tests.
type Memory struct {
	mu   sync.Mutex
	keys map[string]string
}

// NewMemory returns an empty Memory.
func NewMemory() *Memory { return &Memory{keys: map[string]string{}} }

func (m *Memory) Get(model string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k, ok := m.keys[model]
	if !ok {
		return "", ErrNotFound
	}
	return k, nil
}

func (m *Memory) Set(model, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.keys[model] = key
	return nil
}

func (m *Memory) Delete(model string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.keys, model)
	return nil
}
