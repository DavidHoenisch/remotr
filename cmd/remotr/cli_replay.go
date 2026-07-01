package main

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/urfave/cli/v3"
)

type replayState struct {
	active      bool
	cmd         *cli.Command
	positionals []string
	flags       map[string]string
}

var interactiveReplay replayState

func replayReset() {
	interactiveReplay = replayState{}
}

func replayActivate(c *cli.Command) {
	if c == nil {
		return
	}
	if !interactiveReplay.active {
		interactiveReplay.cmd = c
	}
	interactiveReplay.active = true
}

func replayAddPositional(values ...string) {
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			interactiveReplay.positionals = append(interactiveReplay.positionals, v)
		}
	}
}

func replayAddFlag(name, value string) {
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	if name == "" || value == "" {
		return
	}
	if interactiveReplay.flags == nil {
		interactiveReplay.flags = make(map[string]string)
	}
	interactiveReplay.flags[name] = value
}

func writeReplayHintIfNeeded() {
	if !interactiveReplay.active || interactiveReplay.cmd == nil {
		return
	}
	line := formatReplayCommand(interactiveReplay.cmd, interactiveReplay.positionals, interactiveReplay.flags)
	if line == "" {
		return
	}
	writeInfoLine("non-interactive: %s", line)
}

func formatReplayCommand(c *cli.Command, injectPos []string, injectFlags map[string]string) string {
	if c == nil {
		return ""
	}
	lineage := c.Lineage()
	if len(lineage) == 0 {
		return ""
	}
	root := lineage[len(lineage)-1]

	var parts []string
	parts = append(parts, cliExecutableName())

	for _, name := range sortedStrings(root.LocalFlagNames()) {
		if skipReplayFlag(name) {
			continue
		}
		if root.IsSet(name) {
			parts = append(parts, formatSetFlagArgs(root, name)...)
		}
	}

	for _, name := range c.Path()[1:] {
		parts = append(parts, name)
	}

	for i := len(lineage) - 2; i >= 0; i-- {
		cmd := lineage[i]
		for _, name := range sortedStrings(cmd.LocalFlagNames()) {
			if skipReplayFlag(name) {
				continue
			}
			if cmd.IsSet(name) {
				parts = append(parts, formatSetFlagArgs(cmd, name)...)
			}
		}
	}

	for _, name := range sortedStrings(mapKeys(injectFlags)) {
		if c.IsSet(name) {
			continue
		}
		parts = append(parts, "--"+name, shellQuote(injectFlags[name]))
	}

	for _, p := range injectPos {
		parts = append(parts, shellQuote(p))
	}
	for _, arg := range c.Args().Slice() {
		parts = append(parts, shellQuote(arg))
	}

	return strings.Join(parts, " ")
}

func cliExecutableName() string {
	return "remotr"
}

func skipReplayFlag(name string) bool {
	switch name {
	case "help", "h", "version", "v", "generate-shell-completion":
		return true
	default:
		return false
	}
}

func formatSetFlagArgs(cmd *cli.Command, name string) []string {
	switch v := cmd.Value(name).(type) {
	case bool:
		if !v {
			return nil
		}
		return []string{"--" + name}
	case time.Duration:
		return []string{"--" + name, shellQuote(v.String())}
	case []string:
		var out []string
		for _, item := range v {
			out = append(out, "--"+name, shellQuote(item))
		}
		return out
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{"--" + name, shellQuote(v)}
	default:
		if v == nil {
			return nil
		}
		s := strings.TrimSpace(fmt.Sprint(v))
		if s == "" {
			return nil
		}
		return []string{"--" + name, shellQuote(s)}
	}
}

func shellQuote(s string) string {
	if s == "" {
		return `''`
	}
	if shellWordSafe(s) {
		return s
	}
	return `'` + strings.ReplaceAll(s, `'`, `'\''`) + `'`
}

func shellWordSafe(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == '-', r == '_', r == '.', r == '/', r == ':', r == '@':
		default:
			return false
		}
	}
	return true
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func mapKeys(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
