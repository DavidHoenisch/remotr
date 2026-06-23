package main

import "strings"

func withExamples(description string, examples ...string) string {
	if len(examples) == 0 {
		return description
	}
	var b strings.Builder
	if description != "" {
		b.WriteString(description)
		b.WriteString("\n\n")
	}
	b.WriteString("Examples:\n")
	for _, ex := range examples {
		b.WriteString("  ")
		b.WriteString(ex)
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}
