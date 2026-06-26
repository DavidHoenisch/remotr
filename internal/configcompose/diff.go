package configcompose

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/DavidHoenisch/remotr/internal/models"
)

// Diff is a composed-vs-current artifact difference for dry-run output.
type Diff struct {
	Path string `json:"path"`
	Text string `json:"text"`
}

func lineDiff(path string, current, composed []byte) string {
	if len(current) == 0 {
		return diffAllAdded(path, composed)
	}
	curNorm := normalizeYAML(current)
	newNorm := normalizeYAML(composed)
	if bytes.Equal(curNorm, newNorm) {
		return ""
	}
	return unifiedLineDiff(path, curNorm, newNorm)
}

func diffAllAdded(path string, composed []byte) string {
	newLines := splitLines(composed)
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s (missing)\n", path)
	fmt.Fprintf(&b, "+++ %s (composed)\n", path)
	for _, line := range newLines {
		fmt.Fprintf(&b, "+%s\n", line)
	}
	return strings.TrimRight(b.String(), "\n")
}

func unifiedLineDiff(path string, current, composed []byte) string {
	curLines := splitLines(current)
	newLines := splitLines(composed)

	prefix := 0
	for prefix < len(curLines) && prefix < len(newLines) && curLines[prefix] == newLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(curLines)-prefix && suffix < len(newLines)-prefix &&
		curLines[len(curLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}

	oldMid := curLines[prefix : len(curLines)-suffix]
	newMid := newLines[prefix : len(newLines)-suffix]
	if len(oldMid) == 0 && len(newMid) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s (current)\n", path)
	fmt.Fprintf(&b, "+++ %s (composed)\n", path)
	oldStart := prefix + 1
	oldEnd := len(curLines) - suffix
	newStart := prefix + 1
	newEnd := len(newLines) - suffix
	if oldEnd < oldStart {
		oldStart = prefix
		oldEnd = prefix
	}
	if newEnd < newStart {
		newStart = prefix
		newEnd = prefix
	}
	fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", oldStart, len(oldMid), newStart, len(newMid))
	for _, line := range oldMid {
		fmt.Fprintf(&b, "-%s\n", line)
	}
	for _, line := range newMid {
		fmt.Fprintf(&b, "+%s\n", line)
	}
	return strings.TrimRight(b.String(), "\n")
}

func splitLines(data []byte) []string {
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil
	}
	return strings.Split(text, "\n")
}

func lineDiffCron(path string, current, composed []byte) string {
	curNorm := normalizeCronYAML(current)
	newNorm := normalizeCronYAML(composed)
	if bytes.Equal(curNorm, newNorm) {
		return ""
	}
	if len(current) == 0 {
		return diffAllAdded(path, newNorm)
	}
	return unifiedLineDiff(path, curNorm, newNorm)
}

func normalizeCronYAML(data []byte) []byte {
	state, err := models.ParseCronState(bytes.NewReader(data))
	if err != nil {
		return data
	}
	out, err := marshalCronState(state)
	if err != nil {
		return data
	}
	return out
}
