// cida-next is a menu bar app: a panel that hands the text at hand to a model
// and renders the answer as Markdown. Run with a subcommand, the same
// executable is its command line.
package main

import (
	"fmt"
	"os"

	"github.com/egoist/mygo"

	"github.com/Umupa/cida-next/internal/app"
	"github.com/Umupa/cida-next/internal/cli"
)

func main() {
	if len(os.Args) > 1 && cli.Handles(os.Args[1]) {
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(cli.Run(&cli.Env{
			ConfigDir: dir,
			Stdin:     os.Stdin,
			Stdout:    os.Stdout,
			Stderr:    os.Stderr,
		}, os.Args[1:]))
	}
	if err := app.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
