package files

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
)

// applyLineEdit replaces lines matching lineRe with replacement, or appends replacement if no line matched.
func applyLineEdit(content string, lineRe *regexp.Regexp, replacement string) (string, bool, error) {
	replacement = strings.TrimRight(replacement, "\n")
	if replacement == "" {
		return "", false, fmt.Errorf("line replacement content is empty")
	}

	lines := strings.Split(content, "\n")
	trailingNewline := strings.HasSuffix(content, "\n")
	replaced := false

	var out []string
	for i, line := range lines {
		isLast := i == len(lines)-1
		if isLast && line == "" && trailingNewline {
			continue
		}
		if lineRe.MatchString(line) {
			out = append(out, replacement)
			replaced = true
			continue
		}
		out = append(out, line)
	}
	if !replaced {
		out = append(out, replacement)
	}
	result := strings.Join(out, "\n")
	if trailingNewline || len(content) == 0 || strings.HasSuffix(content, "\n") {
		result += "\n"
	}
	return result, replaced, nil
}

func lineReplacePattern(f models.File) (*regexp.Regexp, error) {
	pat := strings.TrimSpace(f.ReplaceRegx)
	if pat == "" {
		pat = strings.TrimSpace(f.WithRegx)
	}
	if pat == "" {
		return nil, fmt.Errorf("line replace requires withRegx or replaceRegx")
	}
	return regexp.Compile(pat)
}
