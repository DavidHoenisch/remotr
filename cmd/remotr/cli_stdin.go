package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/urfave/cli/v3"
)

func readFlagOrStdin(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", fmt.Errorf("read stdin: %w", err)
		}
		return strings.TrimSpace(string(data)), nil
	}
	return raw, nil
}

func readLineFromStdin() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return "", err
		}
		return "", fmt.Errorf("empty stdin")
	}
	return strings.TrimSpace(scanner.Text()), nil
}

func effectiveQuiet(c *cli.Command) bool {
	return c.Bool("quiet") || shouldDefaultQuiet(c)
}

func runWithSpinner(ctx context.Context, c *cli.Command, label string, fn func(context.Context) error) error {
	if !isStderrTerminal() || resolveFormat(c) == formatJSON || c.Bool("json") {
		return fn(ctx)
	}

	done := make(chan struct{})
	var runErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		frames := []string{"|", "/", "-", "\\"}
		i := 0
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "\r%s %s", frames[i%len(frames)], label)
				i++
			}
		}
	}()

	runErr = fn(ctx)
	close(done)
	wg.Wait()
	fmt.Fprintf(os.Stderr, "\r%s\r", strings.Repeat(" ", len(label)+4))
	if runErr != nil {
		return runErr
	}
	return nil
}
