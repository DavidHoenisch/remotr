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

// Owner sets POSIX ownership after writing a file (optional).
type Owner struct {
	UID int
	GID int
}

type Applicator struct {
	File  models.File
	Owner *Owner
}

func New(f models.File) *Applicator {
	return &Applicator{File: f}
}

// NewOwned returns an applicator that chowns the path to uid/gid after apply and revert.
func NewOwned(f models.File, uid, gid int) *Applicator {
	return &Applicator{
		File:  f,
		Owner: &Owner{UID: uid, GID: gid},
	}
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
			if a.File.Content != "" || strings.TrimSpace(a.File.WithRegx) != "" {
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
	var existing []byte
	if _, err := os.Stat(path); err == nil {
		existing, err = os.ReadFile(path) // #nosec G304
		if err != nil {
			return err
		}
		if err := os.WriteFile(bak, existing, 0o600); err != nil { // #nosec G703 -- path validated absolute
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
	body, err := a.applyBody(string(existing))
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, body, mode); err != nil { // #nosec G306 -- mode from desired state
		return err
	}
	return a.chown(path)
}

func (a *Applicator) chown(path string) error {
	if a.Owner == nil {
		return nil
	}
	return os.Chown(path, a.Owner.UID, a.Owner.GID)
}

func (a *Applicator) applyBody(existing string) ([]byte, error) {
	if a.File.UpdateExisting && strings.TrimSpace(a.File.WithRegx) != "" {
		lineRe, err := lineReplacePattern(a.File)
		if err != nil {
			return nil, err
		}
		updated, _, err := applyLineEdit(existing, lineRe, a.File.Content)
		if err != nil {
			return nil, err
		}
		return []byte(updated), nil
	}
	if a.File.Content == "" {
		return nil, fmt.Errorf("file %q: content required", a.File.Name)
	}
	return []byte(a.File.Content), nil
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
	if err := a.chown(path); err != nil {
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
