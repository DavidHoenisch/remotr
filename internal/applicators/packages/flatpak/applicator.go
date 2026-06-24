package flatpak

import (
	"context"
	"fmt"
	"strings"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const (
	DefaultRemote    = "flathub"
	DefaultRemoteURL = "https://flathub.org/repo/flathub.flatpakrepo"
)

// Applicator manages Flatpak applications.
type Applicator struct {
	Package models.Package
	Exec    executil.Runner
}

func New(pkg models.Package, exec executil.Runner) *Applicator {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	return &Applicator{Package: pkg, Exec: exec}
}

func (a *Applicator) Name() string {
	return "flatpak:" + a.remote() + ":" + a.Package.Name
}

func (a *Applicator) Description() string {
	return fmt.Sprintf("flatpak app %s (remote %s)", a.Package.Name, a.remote())
}

func (a *Applicator) remote() string {
	if r := strings.TrimSpace(a.Package.FlatpakRemote); r != "" {
		return r
	}
	return DefaultRemote
}

func (a *Applicator) remoteURL() string {
	if u := strings.TrimSpace(a.Package.FlatpakRemoteURL); u != "" {
		return u
	}
	if a.remote() == DefaultRemote {
		return DefaultRemoteURL
	}
	return ""
}

func (a *Applicator) installed() bool {
	_, _, err := a.Exec.Run("flatpak", "info", a.Package.Name)
	return err == nil
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	inst := a.installed()
	if a.Package.Present {
		return inst, inst
	}
	return inst, !inst
}

func (a *Applicator) Apply(_ context.Context) error {
	_, met := a.State(context.Background())
	if met {
		return appErr.ErrStateAlreadyMet
	}
	if a.Package.Present {
		if err := a.ensureRemote(); err != nil {
			return err
		}
		_, _, err := a.Exec.Run("flatpak", "install", "--assumeyes", "--noninteractive", a.remote(), a.Package.Name)
		return err
	}
	_, _, err := a.Exec.Run("flatpak", "uninstall", "--assumeyes", "--noninteractive", a.Package.Name)
	return err
}

func (a *Applicator) Revert(_ context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) ensureRemote() error {
	remote := a.remote()
	stdout, _, err := a.Exec.Run("flatpak", "remote-list", "--columns=name")
	if err != nil {
		return err
	}
	for _, line := range strings.Split(string(stdout), "\n") {
		if strings.TrimSpace(line) == remote {
			return nil
		}
	}
	url := a.remoteURL()
	if url == "" {
		return fmt.Errorf("flatpak remote %q is not configured and flatpakRemoteURL is required", remote)
	}
	_, _, err = a.Exec.Run("flatpak", "remote-add", "--if-not-exists", remote, url)
	return err
}
