package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/Umupa/cida-next/internal/config"
	"github.com/Umupa/cida-next/internal/keyring"
)

// maxKeyBytes bounds an API key after trimming, so that pointing --file at the
// wrong file cannot fill the keychain with megabytes.
const maxKeyBytes = 2560

// keyUsage is what an unknown or missing key subcommand prints.
const keyUsage = `用法：cida-next key <子命令> [参数]

子命令：
  key set <模型名> (--stdin | --file 路径 | --env 名称) [--json]   把 API Key 存进钥匙串
  key unset <模型名> [--json]                                    删除钥匙串里的 API Key

API Key 只从 --stdin、--file 或 --env 读取，不直接写在命令行上。
`

// keyOptions is the parsed command line of key set or key unset.
type keyOptions struct {
	stdin   bool
	file    string
	fileSet bool
	env     string
	envSet  bool
	json    bool
	// args are the positional arguments in the order they appeared.
	args []string
}

// runKey dispatches the key subcommands.
func runKey(env *Env, args []string) int {
	if len(args) == 0 {
		fmt.Fprint(env.Stderr, keyUsage)
		return ExitUsage
	}
	switch args[0] {
	case "set":
		return runKeySet(env, args[1:])
	case "unset":
		return runKeyUnset(env, args[1:])
	default:
		fmt.Fprint(env.Stderr, keyUsage)
		return ExitUsage
	}
}

// parseKeyOptions parses flags that may come before or after the model name.
// The flag package stops at the first positional, so parsing resumes after
// each one until every argument has been consumed.
func parseKeyOptions(env *Env, name string, args []string, withInput bool) (keyOptions, error) {
	var opts keyOptions
	fs := flag.NewFlagSet("cida-next key "+name, flag.ContinueOnError)
	// The flag package quotes rejected values, and a key pasted as one, as
	// in --stdin=sk-…, must not reach the terminal.
	fs.SetOutput(io.Discard)
	if withInput {
		fs.BoolVar(&opts.stdin, "stdin", false, "从标准输入读取 API Key")
		fs.StringVar(&opts.file, "file", "", "从文件读取 API Key")
		fs.StringVar(&opts.env, "env", "", "从环境变量读取 API Key")
	}
	fs.BoolVar(&opts.json, "json", false, "以 JSON 输出结果")

	rest := args
	for {
		if err := fs.Parse(rest); err != nil {
			return opts, err
		}
		rest = fs.Args()
		if len(rest) == 0 {
			break
		}
		opts.args = append(opts.args, rest[0])
		rest = rest[1:]
	}
	if withInput {
		fs.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "file":
				opts.fileSet = true
			case "env":
				opts.envSet = true
			}
		})
	}
	return opts, nil
}

// runKeySet stores one model's API key in the keychain.
func runKeySet(env *Env, args []string) int {
	opts, err := parseKeyOptions(env, "set", args, true)
	if err != nil {
		return keyParseFailure(env, args, err)
	}
	if len(opts.args) > 1 {
		return keyFail(env, opts.json, ExitUsage,
			"API Key 只从 --stdin、--file 或 --env 读取，不直接写在命令行上，以免留在 shell 历史和 agent 的对话记录里。")
	}
	if len(opts.args) == 0 || opts.args[0] == "" {
		return keyFail(env, opts.json, ExitUsage,
			"需要模型名，例如：cida-next key set <模型名> --stdin。")
	}
	model := opts.args[0]

	sources := 0
	if opts.stdin {
		sources++
	}
	if opts.fileSet {
		sources++
	}
	if opts.envSet {
		sources++
	}
	if sources != 1 {
		return keyFail(env, opts.json, ExitUsage,
			"--stdin、--file、--env 必须且只能指定一个。")
	}

	cfg, err := readKeyConfig(env)
	if err != nil {
		return keyFail(env, opts.json, ExitUsage,
			"配置必须先修好，再设置 API Key：\n"+err.Error())
	}
	if _, ok := cfg.Model(model); !ok {
		return keyFail(env, opts.json, ExitUsage, missingModelMessage(cfg, model))
	}

	key, err := readKeyInput(env, opts)
	if err != nil {
		return keyFail(env, opts.json, ExitUsage, err.Error())
	}
	if err := env.Keyring.Set(model, key); err != nil {
		// The keyring's error may wrap the key, so only a fixed message is
		// reported.
		return keyFail(env, opts.json, ExitKeychain,
			fmt.Sprintf("把「%s」的 API Key 存进钥匙串失败。", model))
	}
	return keySuccess(env, opts.json, model, true)
}

// runKeyUnset removes one model's API key. The model need not be in the
// configuration: unset also cleans up keys of models that were removed.
func runKeyUnset(env *Env, args []string) int {
	opts, err := parseKeyOptions(env, "unset", args, false)
	if err != nil {
		return keyParseFailure(env, args, err)
	}
	if len(opts.args) != 1 || opts.args[0] == "" {
		return keyFail(env, opts.json, ExitUsage,
			"需要且只能指定一个模型名，例如：cida-next key unset <模型名>。")
	}
	model := opts.args[0]
	if err := env.Keyring.Delete(model); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return keyFail(env, opts.json, ExitKeychain,
			fmt.Sprintf("删除「%s」的 API Key 失败。", model))
	}
	return keySuccess(env, opts.json, model, false)
}

// readKeyConfig reads and decodes config.json, the source of model names.
func readKeyConfig(env *Env) (*config.Config, error) {
	path := filepath.Join(env.ConfigDir, config.FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读不了配置文件 %s：%w", path, err)
	}
	cfg, err := config.Decode(data)
	if err != nil {
		return nil, fmt.Errorf("配置文件 %s 有问题：\n%w", path, err)
	}
	return cfg, nil
}

// readKeyInput reads the key from exactly one source and trims it. Errors
// never include the value, so a malformed source cannot leak the key.
func readKeyInput(env *Env, opts keyOptions) (string, error) {
	var raw []byte
	switch {
	case opts.stdin:
		data, err := io.ReadAll(env.Stdin)
		if err != nil {
			return "", errors.New("读不了标准输入，API Key 没有写入。")
		}
		raw = data
	case opts.fileSet:
		data, err := os.ReadFile(opts.file)
		if err != nil {
			return "", fmt.Errorf("读不了 Key 文件「%s」，API Key 没有写入。", opts.file)
		}
		raw = data
	case opts.envSet:
		raw = []byte(env.Getenv(opts.env))
	default:
		return "", errors.New("--stdin、--file、--env 必须且只能指定一个。")
	}

	key := strings.TrimSpace(string(raw))
	switch {
	case key == "":
		return "", errors.New("API Key 是空的，没有写入。")
	case len(key) > maxKeyBytes:
		return "", fmt.Errorf("API Key 有 %d 字节，超过了 %d 字节的上限。", len(key), maxKeyBytes)
	}
	return key, nil
}

// missingModelMessage names the configured models so that the caller can pick
// a valid one without opening config.json.
func missingModelMessage(cfg *config.Config, model string) string {
	names := make([]string, 0, len(cfg.Models))
	for _, m := range cfg.Models {
		names = append(names, m.Name)
	}
	if len(names) == 0 {
		return fmt.Sprintf("配置里没有名为「%s」的模型，配置里还没有任何模型。", model)
	}
	slices.Sort(names)
	return fmt.Sprintf("配置里没有名为「%s」的模型，已配置的模型：%s。", model, strings.Join(names, "、"))
}

// keyParseFailure reports a flag parsing error without its details, which may
// quote a key.
func keyParseFailure(env *Env, args []string, err error) int {
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(env.Stdout, keyUsage)
		return ExitOK
	}
	if !wantsJSON(args) {
		fmt.Fprint(env.Stderr, keyUsage)
	}
	return keyFail(env, wantsJSON(args), ExitUsage, "参数不对，请查看上面的用法。")
}

// wantsJSON reports whether args ask for JSON output even though the flag set
// stopped before parsing --json. It only handles --json's own spellings.
func wantsJSON(args []string) bool {
	on := false
	for _, arg := range args {
		if arg == "--" {
			break
		}
		name, value, hasValue := strings.Cut(arg, "=")
		if name != "--json" && name != "-json" {
			continue
		}
		if !hasValue {
			on = true
			continue
		}
		if b, err := strconv.ParseBool(value); err == nil {
			on = b
		}
	}
	return on
}

// keyFail prints a failure in the requested shape and returns code.
func keyFail(env *Env, jsonOut bool, code int, message string) int {
	if jsonOut {
		writeKeyJSON(env.Stdout, keyJSONError{Error: message})
	} else {
		fmt.Fprintln(env.Stderr, message)
	}
	return code
}

// keySuccess prints the success message in the requested shape.
func keySuccess(env *Env, jsonOut bool, model string, stored bool) int {
	if jsonOut {
		writeKeyJSON(env.Stdout, keyJSONResult{Model: model, Stored: stored})
		return ExitOK
	}
	if stored {
		fmt.Fprintf(env.Stdout, "已把「%s」的 API Key 存进钥匙串。\n", model)
	} else {
		fmt.Fprintf(env.Stdout, "已删除「%s」的 API Key。\n", model)
	}
	return ExitOK
}

// keyJSONResult is one line of --json success output.
type keyJSONResult struct {
	Model  string `json:"model"`
	Stored bool   `json:"stored"`
}

// keyJSONError is one line of --json failure output.
type keyJSONError struct {
	Error string `json:"error"`
}

func writeKeyJSON(w io.Writer, v any) {
	data, err := json.Marshal(v)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "%s\n", data)
}
