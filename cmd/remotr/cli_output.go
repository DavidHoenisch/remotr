package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
)

type outputFormat string

const (
	formatTable outputFormat = "table"
	formatJSON  outputFormat = "json"
	formatPlain outputFormat = "plain"
)

func outputFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{Name: "json", Usage: "output JSON (same as --format json)"},
		&cli.StringFlag{
			Name:  "format",
			Value: string(formatTable),
			Usage: "output format: table, plain, json",
		},
		&cli.BoolFlag{Name: "no-headers", Usage: "omit column headers in table format"},
	}
}

func resolveFormat(c *cli.Command) outputFormat {
	if c.Bool("json") {
		return formatJSON
	}
	switch outputFormat(strings.ToLower(strings.TrimSpace(c.String("format")))) {
	case formatJSON:
		return formatJSON
	case formatPlain:
		return formatPlain
	default:
		return formatTable
	}
}

func encodeJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeInfo(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format, args...)
}

func writeInfoLine(format string, args ...any) {
	writeInfo(format+"\n", args...)
}

func printVersionDetails() {
	if commit != "" {
		fmt.Printf("remotr %s (%s", version, commit)
		if date != "" {
			fmt.Printf(", %s", date)
		}
		fmt.Println(")")
		return
	}
	fmt.Printf("remotr %s\n", version)
}
