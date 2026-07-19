//go:build ignore

package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/DavidHoenisch/remotr/internal/performance"
)

func main() {
	limit := flag.Int("limit", 1<<20, "maximum retained bytes")
	flag.Parse()
	input := io.Reader(os.Stdin)
	if flag.NArg() > 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./scripts/performance-sanitize.go [-limit bytes] [input]")
		os.Exit(2)
	}
	if flag.NArg() == 1 {
		file, err := os.Open(flag.Arg(0))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		defer file.Close()
		input = file
	}
	raw, err := io.ReadAll(io.LimitReader(input, int64(*limit)*4))
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if _, err := os.Stdout.Write(performance.SanitizeDiagnostic(raw, *limit)); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
