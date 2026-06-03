package systemduser_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/applicators/systemduser"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func enabled(v bool) *bool { return &v }

func testUsers() []systemduser.InteractiveUser {
	return []systemduser.InteractiveUser{
		{Username: "alice", UID: 1000},
	}
}

func lingerKey(user string) string {
	return fmt.Sprintf("loginctl [show-user %s -p Linger]", user)
}

func sudoSystemctlKey(user string, uid int, args ...string) string {
	runtime := fmt.Sprintf("XDG_RUNTIME_DIR=/run/user/%d", uid)
	base := []string{"-u", user, "env", runtime, "systemctl", "--user"}
	base = append(base, args...)
	return fmt.Sprintf("sudo %v", base)
}

func TestApplicator_State_compliant(t *testing.T) {
	enabledTrue := enabled(true)
	mock := &executil.MockRunner{Next: map[string]executil.MockResult{
		lingerKey("alice"): {Stdout: []byte("Linger=yes\n")},
		sudoSystemctlKey("alice", 1000, "is-enabled", "soc2-idle-lock.service"): {Stdout: []byte("enabled\n")},
		sudoSystemctlKey("alice", 1000, "is-active", "soc2-idle-lock.service"):  {Stdout: []byte("active\n")},
	}}
	a := systemduser.New(models.SystemdUserResource{
		Name:    "soc2-idle-lock",
		Unit:    "soc2-idle-lock.service",
		Users:   "interactive",
		Linger:  true,
		Enabled: enabledTrue,
		Active:  enabledTrue,
	}, mock)
	a.ListUsers = func() ([]systemduser.InteractiveUser, error) { return testUsers(), nil }
	a.PathExists = func(path string) bool { return path == "/run/user/1000" }

	_, met := a.State(context.Background())
	if !met {
		t.Fatal("expected state met")
	}
}

func TestApplicator_State_driftLinger(t *testing.T) {
	mock := &executil.MockRunner{Next: map[string]executil.MockResult{
		lingerKey("alice"): {Stdout: []byte("Linger=no\n")},
	}}
	a := systemduser.New(models.SystemdUserResource{
		Name:   "soc2-idle-lock",
		Unit:   "soc2-idle-lock.service",
		Users:  "interactive",
		Linger: true,
	}, mock)
	a.ListUsers = func() ([]systemduser.InteractiveUser, error) { return testUsers(), nil }

	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected drift when linger disabled")
	}
}

func TestApplicator_State_driftUnitNotEnabled(t *testing.T) {
	enabledTrue := enabled(true)
	mock := &executil.MockRunner{Next: map[string]executil.MockResult{
		sudoSystemctlKey("alice", 1000, "is-enabled", "soc2-idle-lock.service"): {
			Stdout: []byte("disabled\n"),
		},
	}}
	a := systemduser.New(models.SystemdUserResource{
		Name:    "soc2-idle-lock",
		Unit:    "soc2-idle-lock.service",
		Users:   "interactive",
		Enabled: enabledTrue,
	}, mock)
	a.ListUsers = func() ([]systemduser.InteractiveUser, error) { return testUsers(), nil }

	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected drift when unit not enabled")
	}
}

func TestApplicator_State_unitPathMissing(t *testing.T) {
	unitPath := "/etc/systemd/user/soc2-idle-lock.service"
	a := systemduser.New(models.SystemdUserResource{
		Name:     "soc2-idle-lock",
		Unit:     "soc2-idle-lock.service",
		Users:    "interactive",
		UnitPath: unitPath,
	}, &executil.MockRunner{Next: map[string]executil.MockResult{}})
	a.ListUsers = func() ([]systemduser.InteractiveUser, error) { return testUsers(), nil }
	a.PathExists = func(path string) bool { return path != unitPath }

	_, met := a.State(context.Background())
	if met {
		t.Fatal("expected drift when unit path missing")
	}
}

func TestApplicator_Apply_enablesLingerAndUnit(t *testing.T) {
	enabledTrue := enabled(true)
	mock := &executil.MockRunner{Next: map[string]executil.MockResult{
		lingerKey("alice"): {Stdout: []byte("Linger=no\n")},
		"loginctl [enable-linger alice]":                                          {},
		sudoSystemctlKey("alice", 1000, "is-enabled", "soc2-idle-lock.service"):   {Stdout: []byte("disabled\n")},
		sudoSystemctlKey("alice", 1000, "is-active", "soc2-idle-lock.service"):    {Stdout: []byte("inactive\n")},
		sudoSystemctlKey("alice", 1000, "daemon-reload"):                          {},
		sudoSystemctlKey("alice", 1000, "enable", "--now", "soc2-idle-lock.service"): {},
	}}
	a := systemduser.New(models.SystemdUserResource{
		Name:    "soc2-idle-lock",
		Unit:    "soc2-idle-lock.service",
		Users:   "interactive",
		Linger:  true,
		Enabled: enabledTrue,
		Active:  enabledTrue,
	}, mock)
	a.ListUsers = func() ([]systemduser.InteractiveUser, error) { return testUsers(), nil }
	a.PathExists = func(path string) bool {
		return path == "/run/user/1000"
	}
	a.Sleep = func(_ time.Duration) {}

	if err := a.Apply(context.Background()); err != nil {
		t.Fatalf("Apply() = %v", err)
	}
	callKey := func(c executil.MockCall) string {
		return fmt.Sprintf("%s %v", c.Name, c.Args)
	}
	seen := make(map[string]int)
	for _, c := range mock.Calls {
		seen[callKey(c)]++
	}
	for _, key := range []string{
		"loginctl [enable-linger alice]",
		sudoSystemctlKey("alice", 1000, "daemon-reload"),
		sudoSystemctlKey("alice", 1000, "enable", "--now", "soc2-idle-lock.service"),
	} {
		if seen[key] == 0 {
			t.Fatalf("missing call %q; calls = %v", key, mock.Calls)
		}
	}
}
