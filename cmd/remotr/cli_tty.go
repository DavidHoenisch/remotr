package main

import (
	"os"

	"github.com/urfave/cli/v3"
)

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func isInteractive() bool {
	return isTerminal(os.Stdin) && isTerminal(os.Stdout)
}

func isStderrTerminal() bool {
	return isTerminal(os.Stderr)
}

func isStdoutTerminal() bool {
	return isTerminal(os.Stdout)
}

func shouldDefaultQuiet(c *cli.Command) bool {
	if c.Bool("quiet") {
		return true
	}
	return !isStdoutTerminal()
}

func stdinHasData() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode()&os.ModeCharDevice) == 0 && stat.Size() > 0
}
