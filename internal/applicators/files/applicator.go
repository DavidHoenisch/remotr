package files

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const backupSuffix = ".remotr.bak"

type Applicator struct {
	File models.File
}

func New(f models.File) *Applicator {
	return &Applicator{File: f}
}

func (a *Applicator) Name() string { return "file:" + a.File.Name }

func (a *Applicator) Description() string { return "file " + a.File.Path }

func (a *Applicator) path() (string, error) {
	return validateAbsPath(a.File.Path)
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	path, err := a.path()
	if err != nil {
		return nil, false
	}
	content, err := os.ReadFile(path) // #nosec G304 -- absolute path validated
	if err != nil {
		if os.IsNotExist(err) {
			if a.File.Content != "" || a.File.WithRegx != "" {
				return nil, false
			}
			return nil, true
		}
		return nil, false
	}
	if a.File.WithRegx != "" {
		re, err := regexp.Compile(a.File.WithRegx)
		if err != nil {
			return string(content), false
		}
		return string(content), re.Match(content)
	}
	if a.File.Content != "" {
		return string(content), string(content) == a.File.Content
	}
	return string(content), true
}

func (a *Applicator) Apply(_ context.Context) error {
	path, err := a.path()
	if err != nil {
		return err
	}
	_, met := a.State(context.Background())
	if met {
		return appErr.ErrStateAlreadyMet
	}
	bak := path + backupSuffix
	if _, err := os.Stat(path); err == nil {
		data, err := os.ReadFile(path) // #nosec G304
		if err != nil {
			return err
		}
		if err := os.WriteFile(bak, data, 0o600); err != nil { // #nosec G703 -- path validated absolute
			return fmt.Errorf("backup %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if len(a.File.Mode) > 0 {
		mode = os.FileMode(a.File.Mode[0] & 0o777)
	}
	return os.WriteFile(path, []byte(a.File.Content), mode) // #nosec G306 -- mode from desired state
}

func (a *Applicator) Revert(_ context.Context) error {
	path, err := a.path()
	if err != nil {
		return err
	}
	bak := path + backupSuffix
	data, err := os.ReadFile(bak) // #nosec G304
	if err != nil {
		if os.IsNotExist(err) {
			return os.Remove(path)
		}
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil { // #nosec G306 G703 -- restore prior content, validated path
		return err
	}
	return os.Remove(bak)
}

func validateAbsPath(path string) (string, error) {
	clean := filepath.Clean(strings.TrimSpace(path))
	if clean == "" {
		return "", fmt.Errorf("file path is required")
	}
	if !filepath.IsAbs(clean) {
		return "", fmt.Errorf("file path must be absolute")
	}
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("invalid file path")
	}
	return clean, nil
}
