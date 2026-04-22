package main

import (
	"fmt"
	"os"

	"github.com/gqcdm/aiprobe/internal/cli"
)

func main() {
	app := cli.New()
	if err := app.Run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
}
