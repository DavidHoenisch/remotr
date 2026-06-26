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
		new := strings.TrimRight(string(composed), "\n")
		var b strings.Builder
		fmt.Fprintf(&b, "--- %s (missing)\n", path)
		fmt.Fprintf(&b, "+++ %s (composed)\n", path)
		for _, line := range strings.Split(new, "\n") {
			fmt.Fprintf(&b, "+%s\n", line)
		}
		return strings.TrimRight(b.String(), "\n")
	}
	if bytes.Equal(normalizeYAML(current), normalizeYAML(composed)) {
		return ""
	}
	cur := strings.TrimRight(string(current), "\n")
	new := strings.TrimRight(string(composed), "\n")
	var b strings.Builder
	fmt.Fprintf(&b, "--- %s (current)\n", path)
	fmt.Fprintf(&b, "+++ %s (composed)\n", path)
	for _, line := range strings.Split(cur, "\n") {
		fmt.Fprintf(&b, "-%s\n", line)
	}
	if cur != "" && new != "" {
		fmt.Fprintln(&b)
	}
	for _, line := range strings.Split(new, "\n") {
		fmt.Fprintf(&b, "+%s\n", line)
	}
	return strings.TrimRight(b.String(), "\n")
}

func lineDiffCron(path string, current, composed []byte) string {
	curNorm := normalizeCronYAML(current)
	newNorm := normalizeCronYAML(composed)
	if bytes.Equal(curNorm, newNorm) {
		return ""
	}
	return lineDiff(path, curNorm, newNorm)
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
