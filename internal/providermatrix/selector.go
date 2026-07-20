package providermatrix

import (
	"fmt"
	"regexp"
)

var (
	makeSelector   = regexp.MustCompile(`^make:([A-Za-z0-9][A-Za-z0-9_.-]*)$`)
	goTestSelector = regexp.MustCompile(`^go-test:(\./[A-Za-z0-9_./-]+):(\^.+\$)$`)
)

// ResolveSelector converts the matrix selector vocabulary to one exact
// executable and argument vector. It intentionally provides no shell escape
// hatch and does not accept aggregate command lines.
func ResolveSelector(selector string) (string, []string, error) {
	if match := makeSelector.FindStringSubmatch(selector); match != nil {
		return "make", []string{match[1]}, nil
	}
	if match := goTestSelector.FindStringSubmatch(selector); match != nil {
		return "go", []string{"test", "-mod=vendor", match[1], "-run", match[2], "-count=1"}, nil
	}
	return "", nil, fmt.Errorf("selector %q is not an exact make or go-test target", selector)
}
