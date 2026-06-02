package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
	"gopkg.in/yaml.v3"
)

// File is the on-disk YAML shape for operator CLI defaults.
type File struct {
	ServerURL string `yaml:"server_url"`
	StateDir  string `yaml:"state_dir"`
	CA        string `yaml:"ca"`
	Fleet     string `yaml:"fleet"`
	// Present in configuration-repository manifests; not operator CLI settings.
	Kind string `yaml:"kind"`
}

// Settings are resolved defaults for a command (config file + env + flags).
type Settings struct {
	ServerURL  string
	StateDir   string
	CA         string
	Fleet      string
	ConfigPath string
}

// DefaultPath returns the default operator CLI config file path.
func DefaultPath() string {
	if v := strings.TrimSpace(os.Getenv("REMOTR_CONFIG")); v != "" {
		return expandHome(v)
	}
	return filepath.Join(opcreds.DefaultDir(), "config.yaml")
}

// Load reads settings from path. A missing file yields zero values without error.
func Load(path string) (File, error) {
	path = expandHome(strings.TrimSpace(path))
	if path == "" {
		return File{}, errors.New("config path is required")
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return File{}, nil
		}
		return File{}, fmt.Errorf("read config %s: %w", path, err)
	}

	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return File{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	if strings.TrimSpace(f.Kind) == "remotr-config-repo" {
		return File{}, fmt.Errorf("parse config %s: this is a configuration repository manifest (remotr.yaml), not operator CLI config — use ~/.config/remotr/config.yaml (remotr config init) or unset REMOTR_CONFIG", path)
	}
	return f, nil
}

// Resolve merges config file, environment, and explicit flag values.
// Flag values win when non-empty after trimming.
func Resolve(configPath, flagServerURL, flagStateDir, flagCA, flagFleet string) (Settings, error) {
	if strings.TrimSpace(configPath) == "" {
		configPath = DefaultPath()
	} else {
		configPath = expandHome(configPath)
	}

	file, err := Load(configPath)
	if err != nil {
		return Settings{}, err
	}

	s := Settings{
		ServerURL:  firstNonEmpty(flagServerURL, os.Getenv("REMOTR_SERVER_URL"), file.ServerURL),
		StateDir:   firstNonEmpty(flagStateDir, os.Getenv("REMOTR_OPERATOR_STATE_DIR"), file.StateDir, opcreds.DefaultDir()),
		CA:         firstNonEmpty(flagCA, os.Getenv("REMOTR_CA"), file.CA),
		Fleet:      firstNonEmpty(flagFleet, os.Getenv("REMOTR_FLEET"), file.Fleet),
		ConfigPath: configPath,
	}

	s.StateDir = expandHome(s.StateDir)
	s.CA = expandHome(s.CA)

	if s.CA == "" && s.StateDir != "" {
		candidate := filepath.Join(s.StateDir, "ca.crt")
		if _, err := os.Stat(candidate); err == nil {
			s.CA = candidate
		}
	}

	return s, nil
}

// Save writes settings to the default config path (or REMOTR_CONFIG).
func Save(s Settings) error {
	path := DefaultPath()
	if strings.TrimSpace(s.ConfigPath) != "" {
		path = expandHome(s.ConfigPath)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir config dir: %w", err)
	}

	body, err := yaml.Marshal(File{
		ServerURL: strings.TrimSpace(s.ServerURL),
		StateDir:  strings.TrimSpace(s.StateDir),
		CA:        strings.TrimSpace(s.CA),
		Fleet:     strings.TrimSpace(s.Fleet),
	})
	if err != nil {
		return err
	}

	if err := os.WriteFile(path, body, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

func expandHome(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return path
	}
	if path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return home
	}
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
