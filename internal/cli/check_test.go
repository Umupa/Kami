package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/keyring"
	"github.com/Umupa/cida-next/internal/llm"
)

const (
	checkTestAlphaConfig = `{
  "version": 1,
  "models": [
    {
      "name": "alpha",
      "endpoint": "https://alpha.example.com/v1/chat/completions",
      "format": "chat-completions",
      "model": "alpha-model"
    }
  ]
}`

	checkTestTwoModels = `{
  "version": 1,
  "defaultModel": "alpha",
  "models": [
    {
      "name": "alpha",
      "endpoint": "https://alpha.example.com/v1/chat/completions",
      "format": "chat-completions",
      "model": "alpha-model"
    },
    {
      "name": "beta",
      "endpoint": "https://beta.example.com/v1/chat/completions",
      "format": "chat-completions",
      "model": "beta-model"
    }
  ]
}`

	checkTestAuthNoneConfig = `{
  "version": 1,
  "models": [
    {
      "name": "local",
      "endpoint": "http://127.0.0.1:8080/v1/chat/completions",
      "format": "chat-completions",
      "model": "local-model",
      "auth": "none"
    }
  ]
}`
)

type checkTestEnv struct {
	t      *testing.T
	dir    string
	env    *Env
	memory *keyring.Memory
	client *checkFakeClient
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func newCheckTestEnv(t *testing.T) *checkTestEnv {
	t.Helper()
	te := &checkTestEnv{
		t:      t,
		dir:    t.TempDir(),
		memory: keyring.NewMemory(),
		client: newCheckFakeClient(),
	}
	te.env = &Env{
		ConfigDir: te.dir,
		Keyring:   te.memory,
		Stdin:     strings.NewReader(""),
		Stdout:    &te.stdout,
		Stderr:    &te.stderr,
		Getenv:    func(string) string { return "" },
		Client:    te.client,
	}
	return te
}

func (te *checkTestEnv) writeConfig(data string) {
	te.t.Helper()
	if err := os.WriteFile(filepath.Join(te.dir, config.FileName), []byte(data), 0o600); err != nil {
		te.t.Fatalf("write config.json: %v", err)
	}
}

func (te *checkTestEnv) storeKey(model, key string) {
	te.t.Helper()
	if err := te.memory.Set(model, key); err != nil {
		te.t.Fatalf("Memory.Set: %v", err)
	}
}

func (te *checkTestEnv) runCheck(args ...string) int {
	te.t.Helper()
	args = append([]string{"check"}, args...)
	return Run(te.env, args)
}

func (te *checkTestEnv) decodeJSON() checkJSONOutput {
	te.t.Helper()
	out := te.stdout.String()
	if got := strings.Count(out, "\n"); got != 1 {
		te.t.Fatalf("JSON output has %d newlines, want 1", got)
	}
	var got checkJSONOutput
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		te.t.Fatalf("stdout is not JSON: %v", err)
	}
	return got
}

// assertNoSecret checks that the key never reaches any output. It never
// prints the output it inspects.
func (te *checkTestEnv) assertNoSecret(secret string) {
	te.t.Helper()
	if secret == "" {
		return
	}
	combined := te.stdout.String() + te.stderr.String()
	if strings.Contains(combined, secret) {
		te.t.Error("the API key appeared in check output")
	}
}

type checkJSONOutput struct {
	OK     bool             `json:"ok"`
	Models []checkJSONModel `json:"models"`
	Config string           `json:"config"`
}

type checkJSONModel struct {
	Model  string         `json:"model"`
	OK     bool           `json:"ok"`
	Millis int64          `json:"ms"`
	Reply  string         `json:"reply"`
	Error  *checkJSONFail `json:"error"`
}

type checkJSONFail struct {
	Kind    string `json:"kind"`
	Status  int    `json:"status"`
	Message string `json:"message"`
}

type checkFakeClient struct {
	mu       sync.Mutex
	requests []llm.Request
	replies  map[string]string
	errs     map[string]error
	delay    time.Duration
}

func newCheckFakeClient() *checkFakeClient {
	return &checkFakeClient{replies: map[string]string{}, errs: map[string]error{}}
}

func (f *checkFakeClient) answer(model, reply string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.replies[model] = reply
}

func (f *checkFakeClient) failWith(model string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.errs[model] = err
}

func (f *checkFakeClient) Stream(ctx context.Context, req llm.Request, onText func(string)) error {
	f.mu.Lock()
	f.requests = append(f.requests, req)
	reply, hasReply := f.replies[req.Model.Name]
	err := f.errs[req.Model.Name]
	delay := f.delay
	f.mu.Unlock()

	if delay > 0 {
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
	if err != nil {
		return err
	}
	if hasReply && reply != "" {
		onText(reply)
	}
	return nil
}

func (f *checkFakeClient) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func (f *checkFakeClient) request(i int) llm.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests[i]
}

func TestCheckInvalidConfig(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		te := newCheckTestEnv(t)
		te.writeConfig(`{"version":2}`)

		if code := te.runCheck(); code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		if te.stdout.Len() != 0 {
			t.Error("stdout is not empty")
		}
		errOut := te.stderr.String()
		for _, want := range []string{"配置有问题", "version", "必须是 1"} {
			if !strings.Contains(errOut, want) {
				t.Errorf("stderr does not mention %q", want)
			}
		}
		if te.client.count() != 0 {
			t.Errorf("requests = %d, want 0", te.client.count())
		}
	})

	t.Run("json", func(t *testing.T) {
		te := newCheckTestEnv(t)
		te.writeConfig(`{"version":2}`)

		if code := te.runCheck("--json"); code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		got := te.decodeJSON()
		if got.OK {
			t.Error("JSON ok = true, want false")
		}
		if !strings.Contains(got.Config, "version") || !strings.Contains(got.Config, "必须是 1") {
			t.Errorf("JSON config error does not name the problem: %q", got.Config)
		}
		if te.stderr.Len() != 0 {
			t.Error("stderr is not empty")
		}
	})
}

func TestCheckNoModels(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		te := newCheckTestEnv(t)
		te.writeConfig(`{"version":1}`)

		if code := te.runCheck(); code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		if te.stdout.Len() != 0 {
			t.Error("stdout is not empty")
		}
		if !strings.Contains(te.stderr.String(), "还没有模型") {
			t.Error("stderr does not say the config has no models")
		}
		if te.client.count() != 0 {
			t.Errorf("requests = %d, want 0", te.client.count())
		}
	})

	t.Run("json", func(t *testing.T) {
		te := newCheckTestEnv(t)
		te.writeConfig(`{"version":1}`)

		if code := te.runCheck("--json"); code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		got := te.decodeJSON()
		if got.OK {
			t.Error("JSON ok = true, want false")
		}
		if !strings.Contains(got.Config, "还没有模型") {
			t.Errorf("JSON config error = %q, want the no-models message", got.Config)
		}
		if te.stderr.Len() != 0 {
			t.Error("stderr is not empty")
		}
	})
}

func TestCheckUnknownModel(t *testing.T) {
	te := newCheckTestEnv(t)
	te.writeConfig(checkTestTwoModels)

	if code := te.runCheck("gamma"); code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if te.stdout.Len() != 0 {
		t.Error("stdout is not empty")
	}
	errOut := te.stderr.String()
	for _, want := range []string{"gamma", "alpha", "beta"} {
		if !strings.Contains(errOut, want) {
			t.Errorf("stderr does not mention %q", want)
		}
	}
	if te.client.count() != 0 {
		t.Errorf("requests = %d, want 0", te.client.count())
	}
}

func TestCheckRejectsTwoModels(t *testing.T) {
	te := newCheckTestEnv(t)
	te.writeConfig(checkTestTwoModels)

	if code := te.runCheck("alpha", "beta"); code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if te.stdout.Len() != 0 {
		t.Error("stdout is not empty")
	}
	if !strings.Contains(te.stderr.String(), "一次只能检查一个模型") {
		t.Error("stderr does not explain the one-model limit")
	}
	if te.client.count() != 0 {
		t.Errorf("requests = %d, want 0", te.client.count())
	}
}

func TestCheckMissingKey(t *testing.T) {
	te := newCheckTestEnv(t)
	te.writeConfig(checkTestAlphaConfig)

	if code := te.runCheck("--json"); code != ExitCheckFailed {
		t.Fatalf("exit = %d, want %d", code, ExitCheckFailed)
	}
	got := te.decodeJSON()
	if got.OK {
		t.Error("JSON ok = true, want false")
	}
	if len(got.Models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(got.Models))
	}
	fail := got.Models[0]
	if fail.Model != "alpha" {
		t.Errorf("model = %q, want alpha", fail.Model)
	}
	if fail.OK {
		t.Error("model ok = true, want false")
	}
	if fail.Error == nil {
		t.Fatal("model error = nil, want a failure")
	}
	if fail.Error.Kind != "config" {
		t.Errorf("kind = %q, want config", fail.Error.Kind)
	}
	if !strings.Contains(fail.Error.Message, "key set") {
		t.Errorf("message = %q, want a key set instruction", fail.Error.Message)
	}
	if te.client.count() != 0 {
		t.Errorf("requests = %d, want 0", te.client.count())
	}
}

func TestCheckAuthNoneRunsWithoutKey(t *testing.T) {
	te := newCheckTestEnv(t)
	te.writeConfig(checkTestAuthNoneConfig)
	te.client.answer("local", "local reply")

	if code := te.runCheck(); code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(te.stdout.String(), "✓ local：可用") {
		t.Error("stdout does not report the auth-none model as available")
	}
	if te.stderr.Len() != 0 {
		t.Error("stderr is not empty")
	}
	if te.client.count() != 1 {
		t.Fatalf("requests = %d, want 1", te.client.count())
	}
	req := te.client.request(0)
	if req.Model.Name != "local" {
		t.Errorf("request model = %q, want local", req.Model.Name)
	}
	if req.Model.Auth != config.AuthNone {
		t.Errorf("request auth = %q, want %q", req.Model.Auth, config.AuthNone)
	}
	if req.Key != "" {
		t.Error("request carried a key for an auth-none model")
	}
}

func TestCheckAuthNoneReplyNotCorrupted(t *testing.T) {
	te := newCheckTestEnv(t)
	te.writeConfig(checkTestAuthNoneConfig)
	te.client.answer("local", "local reply")

	if code := te.runCheck("--json"); code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	got := te.decodeJSON()
	if len(got.Models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(got.Models))
	}
	if got.Models[0].Reply != "local reply" {
		t.Errorf("reply = %q, want %q", got.Models[0].Reply, "local reply")
	}
}

func TestCheckAllModelsAnswer(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		te := newCheckTestEnv(t)
		te.writeConfig(checkTestTwoModels)
		te.storeKey("alpha", "key-alpha")
		te.storeKey("beta", "key-beta")
		te.client.answer("alpha", "first reply")
		te.client.answer("beta", "second reply")

		if code := te.runCheck(); code != ExitOK {
			t.Fatalf("exit = %d, want %d", code, ExitOK)
		}
		out := te.stdout.String()
		if got := strings.Count(out, "✓ "); got != 2 {
			t.Errorf("stdout has %d check marks, want 2", got)
		}
		for _, want := range []string{
			"✓ alpha：可用",
			"回复「first reply」",
			"✓ beta：可用",
			"回复「second reply」",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("stdout does not contain %q", want)
			}
		}
		if strings.Contains(out, "✗") {
			t.Error("stdout reports a failure")
		}
		if te.stderr.Len() != 0 {
			t.Error("stderr is not empty")
		}
		if te.client.count() != 2 {
			t.Errorf("requests = %d, want 2", te.client.count())
		}
	})

	t.Run("json", func(t *testing.T) {
		te := newCheckTestEnv(t)
		te.writeConfig(checkTestTwoModels)
		te.storeKey("alpha", "key-alpha")
		te.storeKey("beta", "key-beta")
		te.client.answer("alpha", "first reply")
		te.client.answer("beta", "second reply")
		te.client.delay = 5 * time.Millisecond

		if code := te.runCheck("--json"); code != ExitOK {
			t.Fatalf("exit = %d, want %d", code, ExitOK)
		}
		got := te.decodeJSON()
		if !got.OK {
			t.Error("JSON ok = false, want true")
		}
		if len(got.Models) != 2 {
			t.Fatalf("len(models) = %d, want 2", len(got.Models))
		}
		want := []struct {
			model, reply string
		}{
			{"alpha", "first reply"},
			{"beta", "second reply"},
		}
		for i, w := range want {
			m := got.Models[i]
			if m.Model != w.model {
				t.Errorf("models[%d].model = %q, want %q", i, m.Model, w.model)
			}
			if !m.OK {
				t.Errorf("models[%d].ok = false, want true", i)
			}
			if m.Reply != w.reply {
				t.Errorf("models[%d].reply = %q, want %q", i, m.Reply, w.reply)
			}
			if m.Millis <= 0 {
				t.Errorf("models[%d].ms = %d, want a positive duration", i, m.Millis)
			}
			if m.Error != nil {
				t.Errorf("models[%d].error = %+v, want nil", i, m.Error)
			}
		}
		if !strings.Contains(te.stdout.String(), `"ms":`) {
			t.Error("JSON output does not contain the ms field")
		}
		if te.stderr.Len() != 0 {
			t.Error("stderr is not empty")
		}
	})
}

func TestCheckLimitedError(t *testing.T) {
	te := newCheckTestEnv(t)
	te.writeConfig(checkTestAlphaConfig)
	te.storeKey("alpha", "key-alpha")
	te.client.failWith("alpha", &llm.Error{Kind: llm.KindLimited, Status: 429, Detail: "too many requests"})

	if code := te.runCheck("--json"); code != ExitCheckFailed {
		t.Fatalf("exit = %d, want %d", code, ExitCheckFailed)
	}
	got := te.decodeJSON()
	if got.OK {
		t.Error("JSON ok = true, want false")
	}
	if len(got.Models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(got.Models))
	}
	fail := got.Models[0]
	if fail.OK {
		t.Error("model ok = true, want false")
	}
	if fail.Error == nil {
		t.Fatal("model error = nil, want a failure")
	}
	if fail.Error.Kind != "limited" {
		t.Errorf("kind = %q, want limited", fail.Error.Kind)
	}
	if fail.Error.Status != 429 {
		t.Errorf("status = %d, want 429", fail.Error.Status)
	}
}

func TestCheckEmptyReply(t *testing.T) {
	te := newCheckTestEnv(t)
	te.writeConfig(checkTestAlphaConfig)
	te.storeKey("alpha", "key-alpha")

	if code := te.runCheck("--json"); code != ExitCheckFailed {
		t.Fatalf("exit = %d, want %d", code, ExitCheckFailed)
	}
	got := te.decodeJSON()
	if got.OK {
		t.Error("JSON ok = true, want false")
	}
	if len(got.Models) != 1 {
		t.Fatalf("len(models) = %d, want 1", len(got.Models))
	}
	fail := got.Models[0]
	if fail.OK {
		t.Error("model ok = true, want false")
	}
	if fail.Error == nil {
		t.Fatal("model error = nil, want a failure")
	}
	if fail.Error.Kind != "config" {
		t.Errorf("kind = %q, want config", fail.Error.Kind)
	}
	if !strings.Contains(fail.Error.Message, "no text") {
		t.Errorf("message = %q, want the empty-reply message", fail.Error.Message)
	}
}

func TestCheckMasksKey(t *testing.T) {
	t.Run("reply", func(t *testing.T) {
		const secret = "sk-check-secret-reply"
		te := newCheckTestEnv(t)
		te.writeConfig(checkTestAlphaConfig)
		te.storeKey("alpha", secret)
		te.client.answer("alpha", "the key is "+secret)

		if code := te.runCheck(); code != ExitOK {
			t.Fatalf("exit = %d, want %d", code, ExitOK)
		}
		if !strings.Contains(te.stdout.String(), "••••") {
			t.Error("stdout does not mask the key")
		}
		te.assertNoSecret(secret)
		if te.stderr.Len() != 0 {
			t.Error("stderr is not empty")
		}
	})

	t.Run("llm error detail", func(t *testing.T) {
		const secret = "sk-check-secret-detail"
		te := newCheckTestEnv(t)
		te.writeConfig(checkTestAlphaConfig)
		te.storeKey("alpha", secret)
		te.client.failWith("alpha", &llm.Error{Kind: llm.KindConfig, Detail: "rejected key " + secret})

		if code := te.runCheck("--json"); code != ExitCheckFailed {
			t.Fatalf("exit = %d, want %d", code, ExitCheckFailed)
		}
		if !strings.Contains(te.stdout.String(), "••••") {
			t.Error("JSON output does not mask the key")
		}
		te.assertNoSecret(secret)
		if te.stderr.Len() != 0 {
			t.Error("stderr is not empty")
		}
	})

	t.Run("plain error detail", func(t *testing.T) {
		const secret = "sk-check-secret-plain"
		te := newCheckTestEnv(t)
		te.writeConfig(checkTestAlphaConfig)
		te.storeKey("alpha", secret)
		te.client.failWith("alpha", &checkTestError{message: "rejected key " + secret})

		if code := te.runCheck("--json"); code != ExitCheckFailed {
			t.Fatalf("exit = %d, want %d", code, ExitCheckFailed)
		}
		if !strings.Contains(te.stdout.String(), "••••") {
			t.Error("JSON output does not mask the key")
		}
		te.assertNoSecret(secret)
		if te.stderr.Len() != 0 {
			t.Error("stderr is not empty")
		}
	})
}

func TestCheckVerbose(t *testing.T) {
	const secret = "sk-check-secret-verbose"
	te := newCheckTestEnv(t)
	te.writeConfig(checkTestAlphaConfig)
	te.storeKey("alpha", secret)
	te.client.answer("alpha", "ok")

	if code := te.runCheck("--verbose"); code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	out := te.stdout.String()
	for _, want := range []string{
		"→ POST https://alpha.example.com/v1/chat/completions\n",
		"Authorization: Bearer ••••\n",
		"Content-Type: application/json",
		`"model": "alpha-model"`,
		`"content": "hello"`,
		`"stream": true`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("verbose output does not contain %q", want)
		}
	}
	te.assertNoSecret(secret)
	if te.stderr.Len() != 0 {
		t.Error("stderr is not empty")
	}
}

func TestCheckOneModel(t *testing.T) {
	te := newCheckTestEnv(t)
	te.writeConfig(checkTestTwoModels)
	te.storeKey("alpha", "key-alpha")
	te.storeKey("beta", "key-beta")
	te.client.answer("alpha", "alpha reply")
	te.client.answer("beta", "beta reply")

	if code := te.runCheck("beta"); code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	if got := te.client.count(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
	if got := te.client.request(0).Model.Name; got != "beta" {
		t.Errorf("request model = %q, want beta", got)
	}
	out := te.stdout.String()
	if !strings.Contains(out, "✓ beta：可用") {
		t.Error("stdout does not report beta")
	}
	if strings.Contains(out, "alpha") {
		t.Error("stdout mentions alpha, which was not checked")
	}
	if te.stderr.Len() != 0 {
		t.Error("stderr is not empty")
	}
}

// checkTestError is a plain error whose text can carry a secret.
type checkTestError struct {
	message string
}

func (e *checkTestError) Error() string { return e.message }
