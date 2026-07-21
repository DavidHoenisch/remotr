//go:build ignore

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/DavidHoenisch/remotr/internal/evidenceexceptions"
)

var skipPattern = regexp.MustCompile(`\bt\.Skip(?:f)?\(`)
var exceptionPattern = regexp.MustCompile(`test-exception: (EXC-[0-9]+)`)

func main() {
	registry, err := evidenceexceptions.Load("test/evidence-exceptions.yaml", time.Now())
	if err != nil {
		fail(err)
	}
	known := map[string]bool{}
	for _, record := range registry.Records {
		known[record.ID] = true
	}
	if err := filepath.WalkDir(".", func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "vendor" || entry.Name() == "compose") {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		return checkFile(path, known)
	}); err != nil {
		fail(err)
	}
}

func checkFile(path string, known map[string]bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	previous := ""
	scanner := bufio.NewScanner(f)
	for line := 1; scanner.Scan(); line++ {
		text := scanner.Text()
		if strings.Contains(text, "t.Focus(") || strings.Contains(text, "FIt(") || strings.Contains(text, "FDescribe(") {
			return fmt.Errorf("%s:%d: permanent focused-test marker", path, line)
		}
		if match := exceptionPattern.FindStringSubmatch(text); match != nil {
			previous = match[1]
			continue
		}
		if skipPattern.MatchString(text) {
			if !known[previous] {
				return fmt.Errorf("%s:%d: skip lacks owned test-exception", path, line)
			}
		}
		if strings.TrimSpace(text) != "" {
			previous = ""
		}
	}
	return scanner.Err()
}

func fail(err error) { fmt.Fprintln(os.Stderr, "test policy:", err); os.Exit(1) }
