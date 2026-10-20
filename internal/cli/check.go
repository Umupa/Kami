package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/Umupa/Gloss/internal/config"
	"github.com/Umupa/Gloss/internal/llm"
)

// checkTimeout bounds one model's check.
const checkTimeout = 60 * time.Second

// checkResult is one model's check, as --json prints it.
type checkResult struct {
	Model   string     `json:"model"`
	OK      bool       `json:"ok"`
	Millis  int64      `json:"ms"`
	Reply   string     `json:"reply,omitempty"`
	Problem *checkFail `json:"error,omitempty"`
}

type checkFail struct {
	Kind    string `json:"kind"`
	Status  int    `json:"status,omitempty"`
	Message string `json:"message"`
}

// runCheck validates config.json, then sends each model, or the one named, a
// small request: the user's configuration works when every model answers.
func runCheck(env *Env, args []string) int {
	fs := flag.NewFlagSet("Gloss check", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	verbose := fs.Bool("verbose", false, "")
	jsonOut := fs.Bool("json", false, "")
	var names []string
	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			fmt.Fprintln(env.Stderr, "用法：Gloss check [模型名] [--verbose] [--json]")
			return ExitUsage
		}
		if rest = fs.Args(); len(rest) == 0 {
			break
		}
		names, rest = append(names, rest[0]), rest[1:]
	}
	if len(names) > 1 {
		fmt.Fprintln(env.Stderr, "一次只能检查一个模型，省略模型名则检查全部。")
		return ExitUsage
	}

	cfg, err := readConfig(env)
	if err != nil {
		return checkConfigFailed(env, *jsonOut, err)
	}
	models := cfg.Models
	if len(names) == 1 {
		m, ok := cfg.Model(names[0])
		if !ok {
			return checkConfigFailed(env, *jsonOut, errors.New(missingModelMessage(cfg, names[0])))
		}
		models = []config.Model{m}
	}
	if len(models) == 0 {
		return checkConfigFailed(env, *jsonOut, errors.New("配置里还没有模型：在 config.json 的 models 里添加一项。"))
	}

	client := env.Client
	if client == nil {
		client = llm.HTTP{}
	}
	results := make([]checkResult, 0, len(models))
	for _, m := range models {
		r := checkModel(env, client, m, *verbose && !*jsonOut)
		results = append(results, r)
		if !*jsonOut {
			printCheck(env.Stdout, r)
		}
	}
	ok := !slices.ContainsFunc(results, func(r checkResult) bool { return !r.OK })
	if *jsonOut {
		data, _ := json.Marshal(struct {
			OK     bool          `json:"ok"`
			Models []checkResult `json:"models"`
		}{ok, results})
		fmt.Fprintf(env.Stdout, "%s\n", data)
	}
	if !ok {
		return ExitCheckFailed
	}
	return ExitOK
}

func checkModel(env *Env, client llm.Client, m config.Model, verbose bool) checkResult {
	r := checkResult{Model: m.Name}
	key := m.APIKey
	if key == "" && m.Auth != config.AuthNone {
		r.Problem = &checkFail{Kind: "config", Message: fmt.Sprintf("「%s」没有 apiKey：在 config.json 里给这个模型加上 apiKey。", m.Name)}
		return r
	}
	if verbose {
		printRequest(env.Stdout, llm.PingRequest(m, key))
	}
	ctx, cancel := context.WithTimeout(context.Background(), checkTimeout)
	defer cancel()
	reply, took, err := llm.Ping(ctx, client, m, key)
	r.Millis = took.Milliseconds()
	r.Reply = maskKey(reply, key)
	if err != nil {
		r.Problem = &checkFail{Kind: "unknown", Message: err.Error()}
		var le *llm.Error
		if errors.As(err, &le) {
			r.Problem = &checkFail{Kind: le.Kind.String(), Status: le.Status, Message: le.Detail}
		} else if ctx.Err() != nil {
			r.Problem = &checkFail{Kind: "timeout", Message: "the service did not answer within a minute"}
		}
		r.Problem.Message = maskKey(r.Problem.Message, key)
		return r
	}
	r.OK = true
	return r
}

// maskKey hides the key wherever a service echoes it back.
func maskKey(s, key string) string {
	if key == "" {
		return s
	}
	return strings.ReplaceAll(s, key, "••••")
}

func printCheck(w io.Writer, r checkResult) {
	if r.OK {
		fmt.Fprintf(w, "✓ %s：可用，%d ms，回复「%s」\n", r.Model, r.Millis, r.Reply)
		return
	}
	fmt.Fprintf(w, "✗ %s：%s", r.Model, r.Problem.Kind)
	if r.Problem.Status != 0 {
		fmt.Fprintf(w, "（HTTP %d）", r.Problem.Status)
	}
	fmt.Fprintf(w, "\n  %s\n", r.Problem.Message)
}

// printRequest shows what check sends, with the key masked.
func printRequest(w io.Writer, req llm.Request) {
	h, body, err := llm.Preview(req)
	if err != nil {
		fmt.Fprintf(w, "  无法构造请求：%v\n", err)
		return
	}
	fmt.Fprintf(w, "→ POST %s\n", req.Model.Endpoint)
	for _, k := range slices.Sorted(func(yield func(string) bool) {
		for k := range h {
			if !yield(k) {
				return
			}
		}
	}) {
		fmt.Fprintf(w, "  %s: %s\n", k, h.Get(k))
	}
	fmt.Fprintf(w, "%s\n", body)
}

func checkConfigFailed(env *Env, jsonOut bool, err error) int {
	if jsonOut {
		data, _ := json.Marshal(map[string]any{"ok": false, "config": err.Error()})
		fmt.Fprintf(env.Stdout, "%s\n", data)
	} else {
		fmt.Fprintf(env.Stderr, "配置有问题，先修好 config.json：\n%v\n", err)
	}
	return ExitUsage
}
