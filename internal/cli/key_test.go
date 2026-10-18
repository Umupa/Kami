package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/keyring"
)

const keyCLITestConfig = `{
  "version": 1,
  "models": [
    {"name": "alpha", "endpoint": "https://example.com/v1", "format": "chat-completions", "model": "alpha-model"},
    {"name": "beta", "endpoint": "https://example.com/v1", "format": "responses", "model": "beta-model"}
  ]
}`

type keyCLITestEnv struct {
	t      *testing.T
	dir    string
	env    *Env
	memory *keyring.Memory
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func newKeyCLITestEnv(t *testing.T) *keyCLITestEnv {
	t.Helper()
	te := &keyCLITestEnv{t: t, dir: t.TempDir(), memory: keyring.NewMemory()}
	te.env = &Env{
		ConfigDir: te.dir,
		Keyring:   te.memory,
		Stdin:     strings.NewReader(""),
		Stdout:    &te.stdout,
		Stderr:    &te.stderr,
		Getenv:    func(string) string { return "" },
	}
	return te
}

func (te *keyCLITestEnv) writeConfig(data string) {
	te.t.Helper()
	if err := os.WriteFile(filepath.Join(te.dir, config.FileName), []byte(data), 0o600); err != nil {
		te.t.Fatalf("write config.json: %v", err)
	}
}

func (te *keyCLITestEnv) setStdin(data string) {
	te.env.Stdin = strings.NewReader(data)
}

func (te *keyCLITestEnv) store(model, key string) {
	te.t.Helper()
	if err := te.memory.Set(model, key); err != nil {
		te.t.Fatalf("Memory.Set: %v", err)
	}
}

func (te *keyCLITestEnv) run(args ...string) int {
	te.t.Helper()
	return Run(te.env, args)
}

func (te *keyCLITestEnv) runKey(args ...string) int {
	te.t.Helper()
	return te.run(append([]string{"key"}, args...)...)
}

func (te *keyCLITestEnv) assertStored(model, want string) {
	te.t.Helper()
	got, err := te.memory.Get(model)
	if err != nil {
		te.t.Fatalf("Get(%q): %v", model, err)
	}
	if got != want {
		te.t.Errorf("Get(%q) = %q, want the trimmed key", model, got)
	}
}

func (te *keyCLITestEnv) assertNotStored(model string) {
	te.t.Helper()
	if _, err := te.memory.Get(model); !errors.Is(err, keyring.ErrNotFound) {
		te.t.Errorf("Get(%q) = %v, want ErrNotFound", model, err)
	}
}

// assertNoSecret checks the promise that a key never reaches any output. It
// deliberately does not print the output it inspects.
func (te *keyCLITestEnv) assertNoSecret(secret string) {
	te.t.Helper()
	if secret == "" {
		return
	}
	combined := te.stdout.String() + te.stderr.String()
	if strings.Contains(combined, secret) {
		te.t.Error("the API key appeared in command output")
	}
}

func (te *keyCLITestEnv) assertJSONError(contains ...string) {
	te.t.Helper()
	out := te.stdout.String()
	if strings.Count(out, "\n") != 1 {
		te.t.Errorf("JSON output has %d newlines, want exactly 1", strings.Count(out, "\n"))
	}
	var got keyJSONError
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		te.t.Fatalf("stdout is not a JSON error: %v", err)
	}
	for _, want := range contains {
		if !strings.Contains(got.Error, want) {
			te.t.Errorf("JSON error does not mention %q", want)
		}
	}
}

func TestKeyCLISetFromStdin(t *testing.T) {
	const secret = "sk-stdin-secret"
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"after model", []string{"set", "alpha", "--stdin"}},
		{"before model", []string{"set", "--stdin", "alpha"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			te := newKeyCLITestEnv(t)
			te.writeConfig(keyCLITestConfig)
			te.setStdin("\n\t  " + secret + " \t\n")

			if code := te.runKey(tc.args...); code != ExitOK {
				t.Fatalf("exit = %d, want %d", code, ExitOK)
			}
			if got, want := te.stdout.String(), "已把「alpha」的 API Key 存进钥匙串。\n"; got != want {
				t.Errorf("stdout = %q, want %q", got, want)
			}
			if te.stderr.Len() != 0 {
				t.Errorf("stderr is not empty")
			}
			te.assertStored("alpha", secret)
			te.assertNoSecret(secret)
		})
	}
}

func TestKeyCLISetFromFile(t *testing.T) {
	const secret = "sk-file-secret"
	keyFile := filepath.Join(t.TempDir(), "api-key.txt")
	if err := os.WriteFile(keyFile, []byte("  "+secret+"\n"), 0o600); err != nil {
		t.Fatalf("write key file: %v", err)
	}

	for _, tc := range []struct {
		name string
		args []string
	}{
		{"after model", []string{"set", "alpha", "--file", keyFile}},
		{"before model", []string{"set", "--file", keyFile, "alpha"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			te := newKeyCLITestEnv(t)
			te.writeConfig(keyCLITestConfig)

			if code := te.runKey(tc.args...); code != ExitOK {
				t.Fatalf("exit = %d, want %d", code, ExitOK)
			}
			te.assertStored("alpha", secret)
			te.assertNoSecret(secret)
		})
	}
}

func TestKeyCLISetFromEnvReadsOnce(t *testing.T) {
	const secret = "sk-env-secret"
	te := newKeyCLITestEnv(t)
	te.writeConfig(keyCLITestConfig)

	calls := 0
	te.env.Getenv = func(name string) string {
		calls++
		if name != "CIDA_KEY" {
			t.Errorf("Getenv(%q), want CIDA_KEY", name)
		}
		return " \n" + secret + "\t "
	}

	if code := te.runKey("set", "--env", "CIDA_KEY", "alpha"); code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	if calls != 1 {
		t.Errorf("Getenv calls = %d, want 1", calls)
	}
	te.assertStored("alpha", secret)
	te.assertNoSecret(secret)
}

func TestKeyCLISetUnreadableFile(t *testing.T) {
	te := newKeyCLITestEnv(t)
	te.writeConfig(keyCLITestConfig)
	missing := filepath.Join(t.TempDir(), "missing-key")

	if code := te.runKey("set", "alpha", "--file", missing); code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if te.stdout.Len() != 0 {
		t.Errorf("stdout is not empty")
	}
	if !strings.Contains(te.stderr.String(), filepath.Base(missing)) {
		t.Error("stderr does not name the file it could not read")
	}
	te.assertNotStored("alpha")
}

func TestKeyCLISetEmptyEnv(t *testing.T) {
	te := newKeyCLITestEnv(t)
	te.writeConfig(keyCLITestConfig)
	calls := 0
	te.env.Getenv = func(string) string {
		calls++
		return " \n\t"
	}

	if code := te.runKey("set", "alpha", "--env", "MISSING_KEY"); code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if calls != 1 {
		t.Errorf("Getenv calls = %d, want 1", calls)
	}
	te.assertNotStored("alpha")
}

func TestKeyCLISetRequiresOneSource(t *testing.T) {
	cases := [][]string{
		{"set", "alpha"},
		{"set", "alpha", "--stdin", "--env", "CIDA_KEY"},
		{"set", "--stdin", "--file", "key.txt", "--env", "CIDA_KEY", "alpha"},
	}
	for i, args := range cases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			te := newKeyCLITestEnv(t)
			te.writeConfig(keyCLITestConfig)
			te.setStdin("sk-not-used")

			if code := te.runKey(args...); code != ExitUsage {
				t.Fatalf("exit = %d, want %d", code, ExitUsage)
			}
			if te.stdout.Len() != 0 {
				t.Errorf("stdout is not empty")
			}
			if te.stderr.Len() == 0 {
				t.Error("stderr is empty, want a usage error")
			}
			te.assertNotStored("alpha")
			te.assertNoSecret("sk-not-used")
		})
	}
}

func TestKeyCLISetRefusesCommandLineKey(t *testing.T) {
	const secret = "sk-command-line-secret"
	cases := [][]string{
		{"set", "alpha", secret},
		{"set", "--stdin", "alpha", secret},
		{"set", "alpha", secret, "--stdin"},
	}
	for i, args := range cases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			te := newKeyCLITestEnv(t)
			te.writeConfig(keyCLITestConfig)
			te.setStdin("sk-from-stdin")

			if code := te.runKey(args...); code != ExitUsage {
				t.Fatalf("exit = %d, want %d", code, ExitUsage)
			}
			if te.stdout.Len() != 0 {
				t.Errorf("stdout is not empty")
			}
			errOut := te.stderr.String()
			for _, want := range []string{"--stdin", "--file", "--env", "shell", "agent"} {
				if !strings.Contains(errOut, want) {
					t.Errorf("stderr does not mention %q", want)
				}
			}
			te.assertNoSecret(secret)
			te.assertNotStored("alpha")
		})
	}
}

func TestKeyCLISetEmptyKey(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"whitespace", " \n\t\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			te := newKeyCLITestEnv(t)
			te.writeConfig(keyCLITestConfig)
			te.setStdin(tc.input)

			if code := te.runKey("set", "alpha", "--stdin"); code != ExitUsage {
				t.Fatalf("exit = %d, want %d", code, ExitUsage)
			}
			if te.stdout.Len() != 0 {
				t.Errorf("stdout is not empty")
			}
			if !strings.Contains(te.stderr.String(), "空") {
				t.Error("stderr does not say the key is empty")
			}
			te.assertNotStored("alpha")
		})
	}
}

func TestKeyCLISetKeyLength(t *testing.T) {
	t.Run("at limit", func(t *testing.T) {
		secret := strings.Repeat("k", maxKeyBytes)
		te := newKeyCLITestEnv(t)
		te.writeConfig(keyCLITestConfig)
		te.setStdin(secret)

		if code := te.runKey("set", "alpha", "--stdin"); code != ExitOK {
			t.Fatalf("exit = %d, want %d", code, ExitOK)
		}
		te.assertStored("alpha", secret)
		te.assertNoSecret(secret)
	})

	t.Run("over limit", func(t *testing.T) {
		secret := strings.Repeat("k", maxKeyBytes+1)
		te := newKeyCLITestEnv(t)
		te.writeConfig(keyCLITestConfig)
		te.setStdin(secret)

		if code := te.runKey("set", "alpha", "--stdin"); code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		if !strings.Contains(te.stderr.String(), "2560") {
			t.Error("stderr does not mention the limit")
		}
		te.assertNotStored("alpha")
		te.assertNoSecret(secret)
	})
}

func TestKeyCLISetConfigErrors(t *testing.T) {
	const secret = "sk-config-secret"

	t.Run("missing config", func(t *testing.T) {
		te := newKeyCLITestEnv(t)
		te.setStdin(secret)

		if code := te.runKey("set", "alpha", "--stdin"); code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		if te.stdout.Len() != 0 {
			t.Errorf("stdout is not empty")
		}
		errOut := te.stderr.String()
		for _, want := range []string{"配置", "config.json", "修好"} {
			if !strings.Contains(errOut, want) {
				t.Errorf("stderr does not mention %q", want)
			}
		}
		te.assertNotStored("alpha")
		te.assertNoSecret(secret)
	})

	t.Run("invalid config", func(t *testing.T) {
		te := newKeyCLITestEnv(t)
		te.writeConfig(`{"version":2}`)
		te.setStdin(secret)

		if code := te.runKey("set", "alpha", "--stdin"); code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		errOut := te.stderr.String()
		for _, want := range []string{"配置", "version", "必须是 1"} {
			if !strings.Contains(errOut, want) {
				t.Errorf("stderr does not mention %q", want)
			}
		}
		te.assertNotStored("alpha")
		te.assertNoSecret(secret)
	})

	t.Run("malformed config", func(t *testing.T) {
		te := newKeyCLITestEnv(t)
		te.writeConfig(`{`)
		te.setStdin(secret)

		if code := te.runKey("set", "alpha", "--stdin"); code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		if !strings.Contains(te.stderr.String(), "JSON") {
			t.Error("stderr does not mention the JSON syntax error")
		}
		te.assertNotStored("alpha")
		te.assertNoSecret(secret)
	})
}

func TestKeyCLISetUnknownModelListsNames(t *testing.T) {
	const secret = "sk-unknown-model"
	te := newKeyCLITestEnv(t)
	te.writeConfig(keyCLITestConfig)
	te.setStdin(secret)

	if code := te.runKey("set", "gamma", "--stdin"); code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if te.stdout.Len() != 0 {
		t.Errorf("stdout is not empty")
	}
	errOut := te.stderr.String()
	for _, want := range []string{"gamma", "alpha", "beta"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr does not mention %q", want)
		}
	}
	te.assertNotStored("gamma")
	te.assertNoSecret(secret)
}

func TestKeyCLISetUnknownModelWithoutModels(t *testing.T) {
	const secret = "sk-empty-config"
	te := newKeyCLITestEnv(t)
	te.writeConfig(`{"version":1}`)
	te.setStdin(secret)

	if code := te.runKey("set", "alpha", "--stdin"); code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(te.stderr.String(), "还没有任何模型") {
		t.Error("stderr does not say the config has no models")
	}
	te.assertNoSecret(secret)
}

func TestKeyCLISetKeyringFailure(t *testing.T) {
	const secret = "sk-keyring-secret"
	te := newKeyCLITestEnv(t)
	te.writeConfig(keyCLITestConfig)
	te.setStdin(secret)
	fake := &keyFailingKeyring{setErr: fmt.Errorf("cannot store %q", secret)}
	te.env.Keyring = fake

	if code := te.runKey("set", "alpha", "--stdin"); code != ExitKeychain {
		t.Fatalf("exit = %d, want %d", code, ExitKeychain)
	}
	if fake.setCalls != 1 {
		t.Errorf("Set calls = %d, want 1", fake.setCalls)
	}
	if te.stdout.Len() != 0 {
		t.Errorf("stdout is not empty")
	}
	if !strings.Contains(te.stderr.String(), "失败") {
		t.Error("stderr does not report the failure")
	}
	te.assertNoSecret(secret)
}

func TestKeyCLIJSON(t *testing.T) {
	const secret = "sk-json-secret"

	t.Run("set success", func(t *testing.T) {
		te := newKeyCLITestEnv(t)
		te.writeConfig(keyCLITestConfig)
		te.setStdin(secret + "\n")

		if code := te.runKey("set", "alpha", "--stdin", "--json"); code != ExitOK {
			t.Fatalf("exit = %d, want %d", code, ExitOK)
		}
		if got, want := te.stdout.String(), `{"model":"alpha","stored":true}`+"\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
		if te.stderr.Len() != 0 {
			t.Errorf("stderr is not empty")
		}
		te.assertStored("alpha", secret)
		te.assertNoSecret(secret)
	})

	t.Run("unset success", func(t *testing.T) {
		te := newKeyCLITestEnv(t)
		te.store("alpha", secret)

		if code := te.runKey("unset", "--json", "alpha"); code != ExitOK {
			t.Fatalf("exit = %d, want %d", code, ExitOK)
		}
		if got, want := te.stdout.String(), `{"model":"alpha","stored":false}`+"\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
		if te.stderr.Len() != 0 {
			t.Errorf("stderr is not empty")
		}
		te.assertNotStored("alpha")
		te.assertNoSecret(secret)
	})

	t.Run("usage failure", func(t *testing.T) {
		te := newKeyCLITestEnv(t)
		te.writeConfig(keyCLITestConfig)
		te.setStdin(secret)

		if code := te.runKey("set", "--json", "gamma", "--stdin"); code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		te.assertJSONError("gamma", "alpha", "beta")
		if te.stderr.Len() != 0 {
			t.Errorf("stderr is not empty")
		}
		te.assertNoSecret(secret)
	})

	t.Run("config failure stays one line", func(t *testing.T) {
		te := newKeyCLITestEnv(t)
		te.writeConfig(`{"version":2}`)
		te.setStdin(secret)

		if code := te.runKey("set", "--json", "alpha", "--stdin"); code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		te.assertJSONError("配置", "version")
		if te.stderr.Len() != 0 {
			t.Errorf("stderr is not empty")
		}
		te.assertNoSecret(secret)
	})

	t.Run("keyring failure", func(t *testing.T) {
		te := newKeyCLITestEnv(t)
		te.writeConfig(keyCLITestConfig)
		te.setStdin(secret)
		fake := &keyFailingKeyring{setErr: fmt.Errorf("cannot store %q", secret)}
		te.env.Keyring = fake

		if code := te.runKey("set", "alpha", "--stdin", "--json"); code != ExitKeychain {
			t.Fatalf("exit = %d, want %d", code, ExitKeychain)
		}
		te.assertJSONError("失败")
		if te.stderr.Len() != 0 {
			t.Errorf("stderr is not empty")
		}
		te.assertNoSecret(secret)
	})

	t.Run("unset keyring failure", func(t *testing.T) {
		te := newKeyCLITestEnv(t)
		te.env.Keyring = &keyFailingKeyring{deleteErr: errors.New("keychain is locked")}

		if code := te.runKey("unset", "alpha", "--json"); code != ExitKeychain {
			t.Fatalf("exit = %d, want %d", code, ExitKeychain)
		}
		te.assertJSONError("失败")
		if te.stderr.Len() != 0 {
			t.Errorf("stderr is not empty")
		}
	})
}

func TestKeyCLIUnset(t *testing.T) {
	const secret = "sk-unset-secret"

	t.Run("existing key", func(t *testing.T) {
		te := newKeyCLITestEnv(t)
		te.writeConfig(keyCLITestConfig)
		te.store("alpha", secret)

		if code := te.runKey("unset", "alpha"); code != ExitOK {
			t.Fatalf("exit = %d, want %d", code, ExitOK)
		}
		if got, want := te.stdout.String(), "已删除「alpha」的 API Key。\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
		if te.stderr.Len() != 0 {
			t.Errorf("stderr is not empty")
		}
		te.assertNotStored("alpha")
		te.assertNoSecret(secret)
	})

	t.Run("missing key", func(t *testing.T) {
		te := newKeyCLITestEnv(t)

		if code := te.runKey("unset", "ghost"); code != ExitOK {
			t.Fatalf("exit = %d, want %d", code, ExitOK)
		}
		if got, want := te.stdout.String(), "已删除「ghost」的 API Key。\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
		te.assertNotStored("ghost")
	})

	t.Run("keyring reports not found", func(t *testing.T) {
		te := newKeyCLITestEnv(t)
		te.env.Keyring = &keyFailingKeyring{deleteErr: keyring.ErrNotFound}

		if code := te.runKey("unset", "ghost"); code != ExitOK {
			t.Fatalf("exit = %d, want %d", code, ExitOK)
		}
		if got, want := te.stdout.String(), "已删除「ghost」的 API Key。\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
	})

	t.Run("model absent from config", func(t *testing.T) {
		te := newKeyCLITestEnv(t)
		te.writeConfig(keyCLITestConfig)
		te.store("removed", secret)

		if code := te.runKey("unset", "removed"); code != ExitOK {
			t.Fatalf("exit = %d, want %d", code, ExitOK)
		}
		te.assertNotStored("removed")
		te.assertNoSecret(secret)
	})

	t.Run("no config file", func(t *testing.T) {
		te := newKeyCLITestEnv(t)
		te.store("removed", secret)

		if code := te.runKey("unset", "removed", "--json"); code != ExitOK {
			t.Fatalf("exit = %d, want %d", code, ExitOK)
		}
		if got, want := te.stdout.String(), `{"model":"removed","stored":false}`+"\n"; got != want {
			t.Errorf("stdout = %q, want %q", got, want)
		}
		te.assertNotStored("removed")
		te.assertNoSecret(secret)
	})
}

func TestKeyCLIUnsetUsage(t *testing.T) {
	cases := [][]string{
		{"unset"},
		{"unset", "alpha", "beta"},
		{"unset", ""},
	}
	for i, args := range cases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			te := newKeyCLITestEnv(t)
			te.store("alpha", "sk-keep")

			if code := te.runKey(args...); code != ExitUsage {
				t.Fatalf("exit = %d, want %d", code, ExitUsage)
			}
			if te.stdout.Len() != 0 {
				t.Errorf("stdout is not empty")
			}
			if te.stderr.Len() == 0 {
				t.Error("stderr is empty, want a usage error")
			}
			te.assertStored("alpha", "sk-keep")
			te.assertNoSecret("sk-keep")
		})
	}
}

func TestKeyCLIUnsetKeyringFailure(t *testing.T) {
	te := newKeyCLITestEnv(t)
	fake := &keyFailingKeyring{deleteErr: errors.New("keychain is locked")}
	te.env.Keyring = fake

	if code := te.runKey("unset", "alpha"); code != ExitKeychain {
		t.Fatalf("exit = %d, want %d", code, ExitKeychain)
	}
	if fake.deleteCalls != 1 {
		t.Errorf("Delete calls = %d, want 1", fake.deleteCalls)
	}
	if te.stdout.Len() != 0 {
		t.Errorf("stdout is not empty")
	}
	if !strings.Contains(te.stderr.String(), "失败") {
		t.Error("stderr does not report the failure")
	}
}

func TestKeyCLIUsage(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"missing subcommand", []string{"key"}},
		{"unknown subcommand", []string{"key", "bogus"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			te := newKeyCLITestEnv(t)

			if code := te.run(tc.args...); code != ExitUsage {
				t.Fatalf("exit = %d, want %d", code, ExitUsage)
			}
			if te.stdout.Len() != 0 {
				t.Errorf("stdout is not empty")
			}
			errOut := te.stderr.String()
			for _, want := range []string{"key set", "key unset", "--stdin"} {
				if !strings.Contains(errOut, want) {
					t.Errorf("stderr does not mention %q", want)
				}
			}
		})
	}
}

func TestKeyCLIHelpListsCommands(t *testing.T) {
	te := newKeyCLITestEnv(t)

	if code := te.run("help"); code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	out := te.stdout.String()
	for _, want := range []string{
		"  key set <模型名> --stdin|--file 路径|--env 名称   把 API Key 存进钥匙串\n",
		"  key unset <模型名>                               删除钥匙串里的 API Key\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("help does not contain %q", want)
		}
	}
}

// keyFailingKeyring is a tiny fake for the two failure paths; the memory keyring
// covers every success path.
type keyFailingKeyring struct {
	setErr      error
	deleteErr   error
	setCalls    int
	deleteCalls int
}

func (f *keyFailingKeyring) Get(string) (string, error) { return "", keyring.ErrNotFound }

func (f *keyFailingKeyring) Set(string, string) error {
	f.setCalls++
	return f.setErr
}

func (f *keyFailingKeyring) Delete(string) error {
	f.deleteCalls++
	return f.deleteErr
}

func TestKeyCLIFlagErrorsDoNotEchoValues(t *testing.T) {
	const secret = "sk-pasted-as-flag-value"
	cases := [][]string{
		{"set", "alpha", "--stdin=" + secret},
		{"set", "alpha", "--json=" + secret, "--stdin"},
		{"set", "--" + secret, "alpha"},
	}
	for i, args := range cases {
		t.Run(fmt.Sprintf("case %d", i), func(t *testing.T) {
			te := newKeyCLITestEnv(t)
			te.writeConfig(keyCLITestConfig)

			if code := te.runKey(args...); code != ExitUsage {
				t.Fatalf("exit = %d, want %d", code, ExitUsage)
			}
			te.assertNoSecret(secret)
			te.assertNotStored("alpha")
		})
	}
}

func TestKeyCLIHelp(t *testing.T) {
	for _, sub := range []string{"set", "unset"} {
		t.Run(sub, func(t *testing.T) {
			te := newKeyCLITestEnv(t)
			if code := te.runKey(sub, "--help"); code != ExitOK {
				t.Fatalf("exit = %d, want %d", code, ExitOK)
			}
			if !strings.Contains(te.stdout.String(), "key set") {
				t.Error("stdout does not show the key usage")
			}
		})
	}
}
