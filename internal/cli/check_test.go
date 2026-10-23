package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/llm"
)

// checkTestAlphaConfig returns a config.json with the alpha model. apiKey is
// left out when empty, which is what check reports as a missing key.
func checkTestAlphaConfig(apiKey string) string {
	key := ""
	if apiKey != "" {
		key = fmt.Sprintf(",\n    \"apiKey\": %q", apiKey)
	}
	return fmt.Sprintf(`{
  "version": 1,
  "model": {
    "endpoint": "https://alpha.example.com/v1/chat/completions",
    "format": "chat-completions",
    "model": "alpha-model"%s
  }
}`, key)
}

const checkTestAuthNoneConfig = `{
  "version": 1,
  "model": {
    "endpoint": "http://127.0.0.1:8080/v1/chat/completions",
    "format": "chat-completions",
    "model": "local-model",
    "auth": "none"
  }
}`

type checkTestEnv struct {
	t      *testing.T
	dir    string
	env    *Env
	client *checkFakeClient
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func newCheckTestEnv(t *testing.T) *checkTestEnv {
	t.Helper()
	te := &checkTestEnv{
		t:      t,
		dir:    t.TempDir(),
		client: newCheckFakeClient(),
	}
	te.env = &Env{
		ConfigDir: te.dir,
		Stdin:     strings.NewReader(""),
		Stdout:    &te.stdout,
		Stderr:    &te.stderr,
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
	OK     bool           `json:"ok"`
	Model  string         `json:"model"`
	Millis int64          `json:"ms"`
	Reply  string         `json:"reply"`
	Error  *checkJSONFail `json:"error"`
	Config string         `json:"config"`
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
	reply, hasReply := f.replies[req.Model.Model]
	err := f.errs[req.Model.Model]
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

func TestCheckNoModel(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		te := newCheckTestEnv(t)
		te.writeConfig(`{"version":1}`)

		if code := te.runCheck(); code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		if te.stdout.Len() != 0 {
			t.Error("stdout is not empty")
		}
		if !strings.Contains(te.stderr.String(), "还没有配置模型") {
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
		if !strings.Contains(got.Config, "还没有配置模型") {
			t.Errorf("JSON config error = %q, want the no-models message", got.Config)
		}
		if te.stderr.Len() != 0 {
			t.Error("stderr is not empty")
		}
	})
}

func TestCheckTakesNoModelName(t *testing.T) {
	te := newCheckTestEnv(t)
	te.writeConfig(checkTestAlphaConfig("key-alpha"))

	if code := te.runCheck("alpha"); code != ExitUsage {
		t.Fatalf("exit = %d, want %d", code, ExitUsage)
	}
	if !strings.Contains(te.stderr.String(), "用法：Kami check [--verbose] [--json]") {
		t.Error("stderr does not show the usage")
	}
	if te.client.count() != 0 {
		t.Errorf("requests = %d, want 0", te.client.count())
	}
}

func TestCheckModelAnswers(t *testing.T) {
	t.Run("text", func(t *testing.T) {
		te := newCheckTestEnv(t)
		te.writeConfig(checkTestAlphaConfig("key-alpha"))
		te.client.answer("alpha-model", "first reply")

		if code := te.runCheck(); code != ExitOK {
			t.Fatalf("exit = %d, want %d", code, ExitOK)
		}
		if out := te.stdout.String(); !strings.Contains(out, "✓ alpha-model：可用") || !strings.Contains(out, "回复「first reply」") {
			t.Errorf("stdout does not report the model as available: %q", out)
		}
		if te.stderr.Len() != 0 {
			t.Error("stderr is not empty")
		}
	})

	t.Run("json", func(t *testing.T) {
		te := newCheckTestEnv(t)
		te.writeConfig(checkTestAlphaConfig("key-alpha"))
		te.client.answer("alpha-model", "first reply")
		te.client.delay = 5 * time.Millisecond

		if code := te.runCheck("--json"); code != ExitOK {
			t.Fatalf("exit = %d, want %d", code, ExitOK)
		}
		got := te.decodeJSON()
		if !got.OK || got.Model != "alpha-model" || got.Reply != "first reply" || got.Millis <= 0 || got.Error != nil {
			t.Errorf("JSON = %+v, want the model answering", got)
		}
		if te.stderr.Len() != 0 {
			t.Error("stderr is not empty")
		}
	})
}

func TestCheckMissingKey(t *testing.T) {
	te := newCheckTestEnv(t)
	te.writeConfig(checkTestAlphaConfig(""))

	if code := te.runCheck("--json"); code != ExitCheckFailed {
		t.Fatalf("exit = %d, want %d", code, ExitCheckFailed)
	}
	got := te.decodeJSON()
	if got.OK {
		t.Error("JSON ok = true, want false")
	}
	fail := got
	if fail.Model != "alpha-model" {
		t.Errorf("model = %q, want alpha-model", fail.Model)
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
	if !strings.Contains(fail.Error.Message, "apiKey") {
		t.Errorf("message = %q, want an apiKey instruction", fail.Error.Message)
	}
	if te.client.count() != 0 {
		t.Errorf("requests = %d, want 0", te.client.count())
	}
}

func TestCheckAuthNoneRunsWithoutKey(t *testing.T) {
	te := newCheckTestEnv(t)
	te.writeConfig(checkTestAuthNoneConfig)
	te.client.answer("local-model", "local reply")

	if code := te.runCheck(); code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	if !strings.Contains(te.stdout.String(), "✓ local-model：可用") {
		t.Error("stdout does not report the auth-none model as available")
	}
	if te.stderr.Len() != 0 {
		t.Error("stderr is not empty")
	}
	if te.client.count() != 1 {
		t.Fatalf("requests = %d, want 1", te.client.count())
	}
	req := te.client.request(0)
	if req.Model.Model != "local-model" {
		t.Errorf("request model = %q, want local-model", req.Model.Model)
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
	te.client.answer("local-model", "local reply")

	if code := te.runCheck("--json"); code != ExitOK {
		t.Fatalf("exit = %d, want %d", code, ExitOK)
	}
	got := te.decodeJSON()
	if got.Reply != "local reply" {
		t.Errorf("reply = %q, want %q", got.Reply, "local reply")
	}
}

func TestCheckLimitedError(t *testing.T) {
	te := newCheckTestEnv(t)
	te.writeConfig(checkTestAlphaConfig("key-alpha"))
	te.client.failWith("alpha-model", &llm.Error{Kind: llm.KindLimited, Status: 429, Detail: "too many requests"})

	if code := te.runCheck("--json"); code != ExitCheckFailed {
		t.Fatalf("exit = %d, want %d", code, ExitCheckFailed)
	}
	got := te.decodeJSON()
	if got.OK {
		t.Error("JSON ok = true, want false")
	}
	fail := got
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
	te.writeConfig(checkTestAlphaConfig("key-alpha"))

	if code := te.runCheck("--json"); code != ExitCheckFailed {
		t.Fatalf("exit = %d, want %d", code, ExitCheckFailed)
	}
	got := te.decodeJSON()
	if got.OK {
		t.Error("JSON ok = true, want false")
	}
	fail := got
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
		te.writeConfig(checkTestAlphaConfig(secret))
		te.client.answer("alpha-model", "the key is "+secret)

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
		te.writeConfig(checkTestAlphaConfig(secret))
		te.client.failWith("alpha-model", &llm.Error{Kind: llm.KindConfig, Detail: "rejected key " + secret})

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
		te.writeConfig(checkTestAlphaConfig(secret))
		te.client.failWith("alpha-model", &checkTestError{message: "rejected key " + secret})

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
	te.writeConfig(checkTestAlphaConfig(secret))
	te.client.answer("alpha-model", "ok")

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

// checkTestError is a plain error whose text can carry a secret.
type checkTestError struct {
	message string
}

func (e *checkTestError) Error() string { return e.message }
