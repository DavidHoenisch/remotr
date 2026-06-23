package aisetup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Scope selects user-wide or project-local install paths.
type Scope string

const (
	ScopeUser    Scope = "user"
	ScopeProject Scope = "project"
)

// Agent identifies a supported AI coding agent runtime.
type Agent string

const (
	AgentClaude Agent = "claude"
	AgentCursor Agent = "cursor"
	AgentPi     Agent = "pi"
)

const supportedAgentsHelp = "claude, cursor, pi"

// Target describes where a bundle is installed.
type Target struct {
	Agent      Agent
	Scope      Scope
	InstallDir string
}

// InstallManifest is written to the installed bundle directory.
type InstallManifest struct {
	Agent         string `json:"agent"`
	Scope         string `json:"scope"`
	InstallDir    string `json:"install_dir"`
	BundleVersion string `json:"bundle_version"`
	Source        string `json:"source"`
	SourceVersion string `json:"source_version,omitempty"`
	InstalledAt   string `json:"installed_at"`
}

func ParseAgent(raw string) (Agent, error) {
	switch Agent(strings.ToLower(strings.TrimSpace(raw))) {
	case AgentClaude:
		return AgentClaude, nil
	case AgentCursor:
		return AgentCursor, nil
	case AgentPi:
		return AgentPi, nil
	default:
		return "", fmt.Errorf("unsupported agent %q (supported: %s)", raw, supportedAgentsHelp)
	}
}

func ParseScope(raw string) (Scope, error) {
	switch Scope(strings.ToLower(strings.TrimSpace(raw))) {
	case "", ScopeUser:
		return ScopeUser, nil
	case ScopeProject:
		return ScopeProject, nil
	default:
		return "", fmt.Errorf("unsupported scope %q (supported: user, project)", raw)
	}
}

func SupportedAgents() []Agent {
	return []Agent{AgentClaude, AgentCursor, AgentPi}
}

func ResolveTarget(agent Agent, scope Scope) (Target, error) {
	dir, err := installDir(agent, scope)
	if err != nil {
		return Target{}, err
	}
	return Target{
		Agent:      agent,
		Scope:      scope,
		InstallDir: dir,
	}, nil
}

func installDir(agent Agent, scope Scope) (string, error) {
	switch scope {
	case ScopeProject:
		switch agent {
		case AgentClaude:
			return filepath.Join(".claude", "skills", "remotr-agent"), nil
		case AgentCursor:
			return filepath.Join(".cursor", "skills", "remotr-agent"), nil
		case AgentPi:
			return filepath.Join(".pi", "skills", "remotr-agent"), nil
		}
	case ScopeUser:
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("home directory: %w", err)
		}
		switch agent {
		case AgentClaude:
			return filepath.Join(home, ".claude", "skills", "remotr-agent"), nil
		case AgentCursor:
			return filepath.Join(home, ".cursor", "skills", "remotr-agent"), nil
		case AgentPi:
			return filepath.Join(home, ".pi", "agent", "skills", "remotr-agent"), nil
		}
	}
	return "", fmt.Errorf("unsupported agent %q", agent)
}

func (t Target) Installed() (bool, error) {
	if _, err := os.Stat(filepath.Join(t.InstallDir, "SKILL.md")); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func DefaultDisplayPath(agent Agent, scope Scope) (string, error) {
	t, err := ResolveTarget(agent, scope)
	if err != nil {
		return "", err
	}
	if scope == ScopeUser {
		if home, err := os.UserHomeDir(); err == nil {
			return strings.Replace(t.InstallDir, home, "~", 1), nil
		}
	}
	return t.InstallDir, nil
}
