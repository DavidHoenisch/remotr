package systemduser_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
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

func TestApplicator_State_acceptsSystemctlFalseExitStatuses(t *testing.T) {
	falseValue := false
	mock := &executil.MockRunner{Next: map[string]executil.MockResult{
		sudoSystemctlKey("alice", 1000, "is-enabled", "desktop-agent.service"): {Stdout: []byte("disabled\n"), Err: errors.New("exit status 1")},
		sudoSystemctlKey("alice", 1000, "is-active", "desktop-agent.service"):  {Stdout: []byte("inactive\n"), Err: errors.New("exit status 3")},
	}}
	a := systemduser.New(models.SystemdUserResource{
		Name: "desktop-agent", Unit: "desktop-agent.service", Users: "interactive",
		Masked: &falseValue, Enabled: &falseValue, Active: &falseValue,
	}, mock)
	a.ListUsers = func() ([]systemduser.InteractiveUser, error) { return testUsers(), nil }
	if _, compliant := a.State(context.Background()); !compliant {
		t.Fatal("known systemctl false statuses should be compliant, not probe failures")
	}
}

func TestApplicator_Apply_convergesMaskedDisabledStoppedUserService(t *testing.T) {
	masked, disabled, stopped := true, false, false
	mock := &executil.MockRunner{Next: map[string]executil.MockResult{
		sudoSystemctlKey("alice", 1000, "is-enabled", "desktop-agent.service"): {Stdout: []byte("enabled\n")},
		sudoSystemctlKey("alice", 1000, "is-active", "desktop-agent.service"):  {Stdout: []byte("active\n")},
		sudoSystemctlKey("alice", 1000, "daemon-reload"):                       {},
		sudoSystemctlKey("alice", 1000, "disable", "desktop-agent.service"):    {},
		sudoSystemctlKey("alice", 1000, "stop", "desktop-agent.service"):       {},
		sudoSystemctlKey("alice", 1000, "mask", "desktop-agent.service"):       {},
	}}
	a := systemduser.New(models.SystemdUserResource{
		Name: "desktop-agent", Unit: "desktop-agent.service", Users: "interactive",
		Masked: &masked, Enabled: &disabled, Active: &stopped,
	}, mock)
	a.ListUsers = func() ([]systemduser.InteractiveUser, error) { return testUsers(), nil }

	if err := a.Apply(context.Background()); err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, call := range mock.Calls {
		if call.Name != "sudo" || len(call.Args) < 7 {
			continue
		}
		operation := call.Args[6]
		if operation == "disable" || operation == "stop" || operation == "mask" {
			got = append(got, operation+" "+call.Args[7])
		}
	}
	want := []string{"disable desktop-agent.service", "stop desktop-agent.service", "mask desktop-agent.service"}
	if !slices.Equal(got, want) {
		t.Fatalf("mutation order = %v, want %v; calls=%v", got, want, mock.Calls)
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
		lingerKey("alice"):               {Stdout: []byte("Linger=no\n")},
		"loginctl [enable-linger alice]": {},
		sudoSystemctlKey("alice", 1000, "is-enabled", "soc2-idle-lock.service"): {Stdout: []byte("disabled\n")},
		sudoSystemctlKey("alice", 1000, "is-active", "soc2-idle-lock.service"):  {Stdout: []byte("inactive\n")},
		sudoSystemctlKey("alice", 1000, "daemon-reload"):                        {},
		sudoSystemctlKey("alice", 1000, "enable", "soc2-idle-lock.service"):     {},
		sudoSystemctlKey("alice", 1000, "start", "soc2-idle-lock.service"):      {},
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
		sudoSystemctlKey("alice", 1000, "enable", "soc2-idle-lock.service"),
		sudoSystemctlKey("alice", 1000, "start", "soc2-idle-lock.service"),
	} {
		if seen[key] == 0 {
			t.Fatalf("missing call %q; calls = %v", key, mock.Calls)
		}
	}
}
