package app

import (
	"errors"

	"github.com/egoist/mygo"

	"github.com/Umupa/Kami/internal/config"
)

// migrateKeys moves API keys that earlier builds kept in the login keychain
// into config.json, then removes them from the keychain. It runs at launch;
// once every key has moved it finds nothing to do and reads no keychain.
func (a *app) migrateKeys() {
	cfg := a.store.Snapshot().Config
	var names []string
	for _, m := range cfg.Models {
		if m.APIKey == "" && m.Auth != config.AuthNone {
			names = append(names, m.Name)
		}
	}
	if len(names) == 0 {
		return
	}
	keys := map[string]string{}
	for _, name := range names {
		b, err := mygo.Secrets.Get("model/" + name)
		switch {
		case errors.Is(err, mygo.ErrSecretNotFound):
		case err != nil:
			a.log.Warn("migrate key", "model", name, "err", err)
		default:
			keys[name] = string(b)
		}
	}
	if len(keys) == 0 {
		return
	}
	err := config.Edit(a.store.Path(), func(c *config.Config) {
		for i, m := range c.Models {
			if k, ok := keys[m.Name]; ok && m.APIKey == "" {
				c.Models[i].APIKey = k
			}
		}
	})
	if err != nil {
		a.log.Warn("migrate keys", "err", err)
		return
	}
	for name := range keys {
		_ = mygo.Secrets.Delete("model/" + name)
	}
	a.store.Reload()
	a.log.Info("moved keys from the keychain into config.json", "models", len(keys))
}
