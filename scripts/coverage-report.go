//go:build ignore

// coverage-report summarizes a Go coverprofile for CI artifacts. It is kept in
// the repository so coverage exclusions and changed-line calculations are
// reviewable rather than hidden in a hosted service.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"
)

var generatedPrefixes = []string{
	"internal/store/postgres/db/", // sqlc-generated query and model bindings
}

type block struct {
	file       string
	start, end int
	statements int
	count      int
}

type totals struct {
	statements int
	covered    int
}

func main() {
	profile := flag.String("profile", "", "Go coverprofile to summarize")
	base := flag.String("base", "", "optional Git revision used for changed-line coverage")
	flag.Parse()
	if *profile == "" {
		fatal("-profile is required")
	}

	blocks, err := readProfile(*profile)
	if err != nil {
		fatal("read coverprofile: %v", err)
	}

	byPackage := make(map[string]totals)
	for _, b := range blocks {
		packageName := path.Dir(b.file)
		t := byPackage[packageName]
		t.statements += b.statements
		if b.count > 0 {
			t.covered += b.statements
		}
		byPackage[packageName] = t
	}

	packages := make([]string, 0, len(byPackage))
	for packageName := range byPackage {
		packages = append(packages, packageName)
	}
	sort.Strings(packages)

	fmt.Println("# Package coverage (generated sources excluded)")
	fmt.Println("package\tcovered\tstatements\tpercent")
	var overall totals
	for _, packageName := range packages {
		t := byPackage[packageName]
		overall.statements += t.statements
		overall.covered += t.covered
		fmt.Printf("%s\t%d\t%d\t%.1f%%\n", packageName, t.covered, t.statements, percentage(t.covered, t.statements))
	}
	fmt.Printf("total\t%d\t%d\t%.1f%%\n", overall.covered, overall.statements, percentage(overall.covered, overall.statements))

	if *base == "" {
		fmt.Println("\n# Changed-line coverage\nnot computed (no base revision supplied)")
		return
	}

	changed, err := changedLines(*base)
	if err != nil {
		fatal("read changed lines from %s: %v", *base, err)
	}
	covered, coverable := changedCoverage(blocks, changed)
	fmt.Printf("\n# Changed-line coverage\ncovered\tcoverable\tpercent\n%d\t%d\t%.1f%%\n", covered, coverable, percentage(covered, coverable))
}

func readProfile(name string) ([]block, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	unique := make(map[string]block)
	scanner := bufio.NewScanner(f)
	line := 0
	for scanner.Scan() {
		line++
		if line == 1 {
			if !strings.HasPrefix(scanner.Text(), "mode: ") {
				return nil, fmt.Errorf("first line does not declare coverage mode")
			}
			continue
		}
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return nil, fmt.Errorf("line %d: expected three fields", line)
		}
		file, start, end, err := profileRange(fields[0])
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		file = repositoryPath(file)
		if isGenerated(file) {
			continue
		}
		statements, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("line %d: statements: %w", line, err)
		}
		count, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("line %d: count: %w", line, err)
		}
		candidate := block{file: file, start: start, end: end, statements: statements, count: count}
		key := fmt.Sprintf("%s:%d:%d:%d", file, start, end, statements)
		if existing, ok := unique[key]; !ok || candidate.count > existing.count {
			// -coverpkg writes the same block once for every test binary. A
			// block is covered if any of those binaries executed it.
			unique[key] = candidate
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	blocks := make([]block, 0, len(unique))
	for _, b := range unique {
		blocks = append(blocks, b)
	}
	return blocks, nil
}

func profileRange(value string) (string, int, int, error) {
	separator := strings.LastIndex(value, ":")
	if separator == -1 {
		return "", 0, 0, fmt.Errorf("missing source range in %q", value)
	}
	rangeParts := strings.Split(value[separator+1:], ",")
	if len(rangeParts) != 2 {
		return "", 0, 0, fmt.Errorf("invalid source range in %q", value)
	}
	start, err := lineNumber(rangeParts[0])
	if err != nil {
		return "", 0, 0, err
	}
	end, err := lineNumber(rangeParts[1])
	if err != nil {
		return "", 0, 0, err
	}
	return value[:separator], start, end, nil
}

func lineNumber(value string) (int, error) {
	line, _, ok := strings.Cut(value, ".")
	if !ok {
		return 0, fmt.Errorf("missing column in %q", value)
	}
	return strconv.Atoi(line)
}

func repositoryPath(value string) string {
	first := -1
	for _, marker := range []string{"agent/", "cmd/", "internal/"} {
		if index := strings.Index(value, marker); index >= 0 && (first == -1 || index < first) {
			first = index
		}
	}
	if first >= 0 {
		return value[first:]
	}
	return value
}

func isGenerated(file string) bool {
	for _, prefix := range generatedPrefixes {
		if strings.HasPrefix(file, prefix) {
			return true
		}
	}
	return false
}

func changedLines(base string) (map[string]map[int]bool, error) {
	output, err := exec.Command("git", "diff", "--unified=0", base+"...HEAD", "--", ".").Output()
	if err != nil {
		return nil, err
	}

	changed := make(map[string]map[int]bool)
	var current string
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			current = strings.TrimPrefix(line, "+++ b/")
			continue
		}
		if !strings.HasPrefix(line, "@@ ") || !strings.HasSuffix(current, ".go") || isGenerated(current) {
			continue
		}
		start, length, ok := addedRange(line)
		if !ok || length == 0 {
			continue
		}
		if changed[current] == nil {
			changed[current] = make(map[int]bool)
		}
		for lineNumber := start; lineNumber < start+length; lineNumber++ {
			changed[current][lineNumber] = true
		}
	}
	return changed, nil
}

func addedRange(header string) (int, int, bool) {
	parts := strings.Fields(header)
	if len(parts) < 3 {
		return 0, 0, false
	}
	value := strings.TrimPrefix(parts[2], "+")
	value = strings.TrimSuffix(value, " @@")
	start, length, hasLength := strings.Cut(value, ",")
	if !hasLength {
		length = "1"
	}
	startNumber, err := strconv.Atoi(start)
	if err != nil {
		return 0, 0, false
	}
	lengthNumber, err := strconv.Atoi(length)
	if err != nil {
		return 0, 0, false
	}
	return startNumber, lengthNumber, true
}

func changedCoverage(blocks []block, changed map[string]map[int]bool) (int, int) {
	covered, coverable := 0, 0
	for file, lines := range changed {
		for line := range lines {
			for _, b := range blocks {
				if b.file != file || line < b.start || line > b.end {
					continue
				}
				coverable++
				if b.count > 0 {
					covered++
				}
				break
			}
		}
	}
	return covered, coverable
}

func percentage(numerator, denominator int) float64 {
	if denominator == 0 {
		return 100
	}
	return float64(numerator) * 100 / float64(denominator)
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "coverage report: "+format+"\n", args...)
	os.Exit(1)
}
