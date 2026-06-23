package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
)

type colorSetting string

const (
	colorAuto   colorSetting = "auto"
	colorAlways colorSetting = "always"
	colorNever  colorSetting = "never"
)

func colorFlag() cli.Flag {
	return &cli.StringFlag{
		Name:  "color",
		Value: string(colorAuto),
		Usage: "color stderr labels: auto, always, never (respects NO_COLOR)",
	}
}

func colorEnabled(c *cli.Command) bool {
	if c == nil {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if strings.EqualFold(os.Getenv("TERM"), "dumb") {
		return false
	}
	switch colorSetting(strings.ToLower(strings.TrimSpace(c.String("color")))) {
	case colorNever:
		return false
	case colorAlways:
		return true
	default:
		return isStderrTerminal()
	}
}

func labelError(c *cli.Command, text string) string {
	return styleLabel(c, text, "\033[31m", "\033[0m")
}

func labelWarn(c *cli.Command, text string) string {
	return styleLabel(c, text, "\033[33m", "\033[0m")
}

func labelOK(c *cli.Command, text string) string {
	return styleLabel(c, text, "\033[32m", "\033[0m")
}

func styleLabel(c *cli.Command, text, open, close string) string {
	if !colorEnabled(c) {
		return text
	}
	return open + text + close
}

func writeWarn(c *cli.Command, format string, args ...any) {
	writeInfo("%s %s\n", labelWarn(c, "warning:"), fmt.Sprintf(format, args...))
}

func writeOK(c *cli.Command, format string, args ...any) {
	writeInfo("%s %s\n", labelOK(c, "ok:"), fmt.Sprintf(format, args...))
}
