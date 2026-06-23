package agentversion

import (
	"fmt"
	"strconv"
	"strings"
)

// Normalize returns a canonical tag form (with leading v).
func Normalize(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("version is required")
	}
	s = strings.TrimPrefix(s, "v")
	if s == "" {
		return "", fmt.Errorf("invalid version")
	}
	return "v" + s, nil
}

// Match reports whether reported satisfies desired (both normalized).
func Match(desired, reported string) bool {
	d, err := Normalize(desired)
	if err != nil {
		return false
	}
	r, err := Normalize(reported)
	if err != nil {
		return false
	}
	return d == r
}

// Compare reports whether a is less than, equal to, or greater than b using semver core.
// Pre-release segments are ignored; invalid versions return an error.
func Compare(a, b string) (int, error) {
	av, err := coreParts(a)
	if err != nil {
		return 0, err
	}
	bv, err := coreParts(b)
	if err != nil {
		return 0, err
	}
	for i := range av {
		switch {
		case av[i] < bv[i]:
			return -1, nil
		case av[i] > bv[i]:
			return 1, nil
		}
	}
	return 0, nil
}

func coreParts(raw string) ([3]int, error) {
	tag, err := Normalize(raw)
	if err != nil {
		return [3]int{}, err
	}
	tag = strings.TrimPrefix(tag, "v")
	if i := strings.IndexAny(tag, "-+"); i >= 0 {
		tag = tag[:i]
	}
	parts := strings.Split(tag, ".")
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("invalid semver: %q", raw)
	}
	var out [3]int
	for i, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil || n < 0 {
			return [3]int{}, fmt.Errorf("invalid semver segment: %q", part)
		}
		out[i] = n
	}
	return out, nil
}
