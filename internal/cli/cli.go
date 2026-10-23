// Package cli is the command line of the app's executable. It covers only
// what editing config.json cannot: checking that the configuration works.
package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Umupa/Kami/internal/config"
	"github.com/Umupa/Kami/internal/mode"

	"github.com/Umupa/Kami/internal/llm"
)

// Exit codes distinguish invalid configuration from a failed service check.
const (
	ExitOK          = 0
	ExitUsage       = 64 // also: the configuration is invalid
	ExitCheckFailed = 69
)

// Env is what a command runs against.
type Env struct {
	// ConfigDir holds config.json and its schema.
	ConfigDir string
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	// Client sends check's requests; nil uses HTTP.
	Client llm.Client
}

// commands are the subcommands; Handles reports the first argument of one.
var commands = map[string]func(env *Env, args []string) int{
	"help":  runHelp,
	"check": runCheck,
}

// Handles reports whether a first argument is for the command line rather
// than the app. Any word is, so that a mistyped command fails instead of
// launching the app; flags are left to the app, which macOS may pass some to,
// such as -psn_….
func Handles(arg string) bool {
	return !strings.HasPrefix(arg, "-") || arg == "-h" || arg == "--help"
}

// Run runs the subcommand args[0] and returns the exit code.
func Run(env *Env, args []string) int {
	if len(args) == 0 {
		return runHelp(env, nil)
	}
	cmd, ok := commands[args[0]]
	if !ok {
		if args[0] == "-h" || args[0] == "--help" {
			return runHelp(env, nil)
		}
		fmt.Fprintf(env.Stderr, "不认识的命令：%s\n", args[0])
		return ExitUsage
	}
	return cmd(env, args[1:])
}

const usage = `用法：Kami <命令> [参数]

设置都在 config.json 里，直接编辑即可，保存后正在运行的应用立即生效。
字段说明见同目录的 config.schema.json。

命令：
  help                         显示本帮助
  check [--verbose] [--json]   校验配置，并向模型发一个小请求

配置目录：%s
`

func runHelp(env *Env, _ []string) int {
	fmt.Fprintf(env.Stdout, usage, env.ConfigDir)
	return ExitOK
}

// readConfig reads and validates config.json as the app does.
func readConfig(env *Env) (*config.Config, error) {
	path := filepath.Join(env.ConfigDir, config.FileName)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读不了配置文件 %s：%w", path, err)
	}
	cfg, err := config.Decode(data)
	if err == nil {
		err = mode.Check(cfg)
	}
	if err != nil {
		return nil, fmt.Errorf("配置文件 %s 有问题：\n%w", path, err)
	}
	return cfg, nil
}
