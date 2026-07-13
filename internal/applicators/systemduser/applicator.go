package systemduser

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	appErr "github.com/DavidHoenisch/remotr/internal/errors"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

const runtimeWaitTimeout = 15 * time.Second

type Applicator struct {
	Resource   models.SystemdUserResource
	Exec       executil.Runner
	ListUsers  func() ([]InteractiveUser, error)
	PathExists func(string) bool
	Sleep      func(time.Duration)
}

func New(r models.SystemdUserResource, exec executil.Runner) *Applicator {
	if exec == nil {
		exec = executil.OSRunner{}
	}
	return &Applicator{
		Resource: r,
		Exec:     exec,
		Sleep:    time.Sleep,
		PathExists: func(path string) bool {
			_, err := os.Stat(path) // #nosec G703 -- runtime dir under /run/user
			return err == nil
		},
	}
}

func (a *Applicator) Name() string { return "systemdUser:" + a.Resource.Name }

func (a *Applicator) Description() string {
	return "systemd user unit " + a.Resource.Unit
}

func (a *Applicator) users() ([]InteractiveUser, error) {
	fn := a.ListUsers
	if fn == nil {
		fn = ListInteractiveUsers
	}
	return fn()
}

func (a *Applicator) pathExists(path string) bool {
	fn := a.PathExists
	if fn == nil {
		fn = func(p string) bool {
			_, err := os.Stat(p) // #nosec G703 -- runtime dir under /run/user
			return err == nil
		}
	}
	return fn(path)
}

func (a *Applicator) sleep(d time.Duration) {
	fn := a.Sleep
	if fn == nil {
		fn = time.Sleep
	}
	fn(d)
}

func (a *Applicator) State(_ context.Context) (any, bool) {
	if a.Resource.UnitPath != "" && !a.pathExists(a.Resource.UnitPath) {
		return nil, false
	}
	users, err := a.users()
	if err != nil {
		return nil, false
	}
	for _, u := range users {
		if a.Resource.Linger && !a.lingerEnabled(u.Username) {
			return false, false
		}
		if a.Resource.Masked != nil {
			masked, err := a.isMasked(u)
			if err != nil || *a.Resource.Masked != masked {
				return masked, false
			}
		}
		if a.Resource.Enabled != nil {
			enabled, err := a.isEnabled(u)
			if err != nil || *a.Resource.Enabled != enabled {
				return enabled, false
			}
		}
		if a.Resource.Active != nil {
			active, err := a.isActive(u)
			if err != nil || *a.Resource.Active != active {
				return active, false
			}
		}
	}
	return true, true
}

func (a *Applicator) Apply(ctx context.Context) error {
	if _, met := a.State(ctx); met {
		return appErr.ErrStateAlreadyMet
	}
	if a.Resource.UnitPath != "" {
		if !a.pathExists(a.Resource.UnitPath) {
			return fmt.Errorf("unit path does not exist: %s", a.Resource.UnitPath)
		}
	}
	users, err := a.users()
	if err != nil {
		return err
	}
	for _, u := range users {
		if a.Resource.Linger {
			if _, _, err := a.Exec.Run("loginctl", "enable-linger", u.Username); err != nil {
				return err
			}
			if err := a.waitRuntimeDir(u.UID); err != nil {
				return err
			}
		}
		if _, _, err := a.userSystemctl(u, "daemon-reload"); err != nil {
			return err
		}
		if a.Resource.Masked != nil && !*a.Resource.Masked {
			if _, _, err := a.userSystemctl(u, "unmask", a.Resource.Unit); err != nil {
				return err
			}
		}
		if a.Resource.Enabled != nil {
			operation := "disable"
			if *a.Resource.Enabled {
				operation = "enable"
			}
			if _, _, err := a.userSystemctl(u, operation, a.Resource.Unit); err != nil {
				return err
			}
		}
		if a.Resource.Active != nil {
			operation := "stop"
			if *a.Resource.Active {
				operation = "start"
			}
			if _, _, err := a.userSystemctl(u, operation, a.Resource.Unit); err != nil {
				return err
			}
		}
		if a.Resource.Masked != nil && *a.Resource.Masked {
			if _, _, err := a.userSystemctl(u, "mask", a.Resource.Unit); err != nil {
				return err
			}
		}
	}
	return nil
}

func (a *Applicator) Revert(_ context.Context) error { return appErr.ErrNoOp }

func (a *Applicator) runtimeDir(uid int) string {
	return fmt.Sprintf("/run/user/%d", uid)
}

func (a *Applicator) waitRuntimeDir(uid int) error {
	path := a.runtimeDir(uid)
	deadline := time.Now().Add(runtimeWaitTimeout)
	for time.Now().Before(deadline) {
		if a.pathExists(path) {
			return nil
		}
		a.sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %s", path)
}

func (a *Applicator) userSystemctl(u InteractiveUser, args ...string) ([]byte, []byte, error) {
	runtime := fmt.Sprintf("XDG_RUNTIME_DIR=%s", a.runtimeDir(u.UID))
	cmdArgs := append([]string{"-u", u.Username, "env", runtime, "systemctl", "--user"}, args...)
	return a.Exec.Run("sudo", cmdArgs...)
}

func (a *Applicator) lingerEnabled(username string) bool {
	out, _, err := a.Exec.Run("loginctl", "show-user", username, "-p", "Linger")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "Linger=yes"
}

func (a *Applicator) isEnabled(u InteractiveUser) (bool, error) {
	out, _, err := a.userSystemctl(u, "is-enabled", a.Resource.Unit)
	s := strings.TrimSpace(string(out))
	switch s {
	case "enabled", "enabled-runtime":
		return true, nil
	case "disabled", "static", "indirect", "masked", "masked-runtime", "not-found":
		return false, nil
	}
	return false, err
}

func (a *Applicator) isMasked(u InteractiveUser) (bool, error) {
	out, _, err := a.userSystemctl(u, "is-enabled", a.Resource.Unit)
	value := strings.TrimSpace(string(out))
	if value == "masked" || value == "masked-runtime" {
		return true, nil
	}
	switch value {
	case "enabled", "enabled-runtime", "disabled", "static", "indirect", "not-found":
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return false, nil
}

func (a *Applicator) isActive(u InteractiveUser) (bool, error) {
	out, _, err := a.userSystemctl(u, "is-active", a.Resource.Unit)
	value := strings.TrimSpace(string(out))
	if value == "active" {
		return true, nil
	}
	switch value {
	case "inactive", "failed", "unknown", "not-found":
		return false, nil
	}
	return false, err
}
