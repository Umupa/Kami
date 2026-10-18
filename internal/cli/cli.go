// Package cli is the command line of the app's executable. It covers only
// what editing config.json cannot: storing API keys in the keychain and
// checking that the configuration works.
package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/Umupa/cida-next/internal/keyring"
)

// Exit codes, the same as Cida's (configuration.md §二).
const (
	ExitOK          = 0
	ExitUsage       = 64 // also: the configuration is invalid
	ExitCheckFailed = 69
	ExitKeychain    = 74
)

// Env is what a command runs against.
type Env struct {
	// ConfigDir holds config.json and its schema.
	ConfigDir string
	Keyring   keyring.Keyring
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	// Getenv reads the environment, for --env.
	Getenv func(string) string
}

// commands are the subcommands; Handles reports the first argument of one.
var commands = map[string]func(env *Env, args []string) int{
	"help": runHelp,
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

const usage = `用法：cida-next <命令> [参数]

设置都在 config.json 里，直接编辑即可，保存后正在运行的应用立即生效。
字段说明见同目录的 config.schema.json。

命令：
  help                      显示本帮助

配置目录：%s
`

func runHelp(env *Env, _ []string) int {
	fmt.Fprintf(env.Stdout, usage, env.ConfigDir)
	return ExitOK
}
