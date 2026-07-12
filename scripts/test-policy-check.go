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

	"gopkg.in/yaml.v3"
)

type exception struct {
	ID, Kind, Owner, Issue, Reason, Expires string
	EquivalentSelector                      string `yaml:"equivalent_selector"`
}

var skipPattern = regexp.MustCompile(`\bt\.Skip(?:f)?\(`)
var exceptionPattern = regexp.MustCompile(`test-exception: (EXC-[0-9]+)`)

func main() {
	data, err := os.ReadFile("test/evidence-exceptions.yaml")
	if err != nil {
		fail(err)
	}
	var records []exception
	if err := yaml.Unmarshal(data, &records); err != nil {
		fail(err)
	}
	known := map[string]bool{}
	for _, record := range records {
		known[record.ID] = true
		expires, err := time.Parse("2006-01-02", record.Expires)
		if record.Owner == "" || record.Issue == "" || record.Reason == "" || err != nil || !expires.After(time.Now()) || (record.Kind == "quarantine" && record.EquivalentSelector == "") {
			fail(fmt.Errorf("invalid or expired exception %s", record.ID))
		}
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
