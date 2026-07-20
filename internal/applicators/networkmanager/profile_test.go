package networkmanager

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/agent/networkstate"
	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

func TestProfileProbeFailureIncludesBoundedNetworkManagerDiagnostic(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nmcli [-t -f GENERAL.DEVICE,GENERAL.TYPE,GENERAL.HWADDR device show]": {
			Stdout: []byte("GENERAL.DEVICE:eth0\nGENERAL.TYPE:ethernet\nGENERAL.HWADDR:02:00:00:00:00:01\n"),
		},
		"nmcli [-t -f GENERAL.CONNECTION device show eth0]": {Stdout: []byte("GENERAL.CONNECTION:office\n")},
		"nmcli [-t -f connection.id,connection.type,connection.autoconnect,802-3-ethernet.mtu,ipv4.method,ipv6.method,ipv4.addresses,ipv6.addresses connection show office]": {
			Stderr: []byte("Error: invalid field '802-3-ethernet.mtu'\n"), Err: errors.New("exit status 2"),
		},
	}}
	provider := NewProfile(models.NetworkProfileResource{
		Name: "uplink", Provider: models.NetworkProviderNetworkManager,
		Selector: models.NetworkInterfaceSelector{Name: "eth0"}, ProfileName: "office", ProfileType: "ethernet",
	}, runner)

	check := provider.Check(context.Background())
	if check.Status != executor.CheckFailed || check.Err == nil || !strings.Contains(check.Err.Error(), "invalid field '802-3-ethernet.mtu'") {
		t.Fatalf("profile probe failure = %+v", check)
	}
}

type acknowledgedConvergenceRunner struct {
	checkpoint string
	active     bool
	calls      []executil.MockCall
}

func (r *acknowledgedConvergenceRunner) Run(name string, args ...string) ([]byte, []byte, error) {
	r.calls = append(r.calls, executil.MockCall{Name: name, Args: append([]string(nil), args...)})
	switch {
	case name == "nmcli" && slices.Equal(args, []string{"-t", "-f", "GENERAL.DEVICE,GENERAL.TYPE,GENERAL.HWADDR", "device", "show"}):
		return []byte("GENERAL.DEVICE:eth0\nGENERAL.TYPE:ethernet\nGENERAL.HWADDR:02:00:00:00:00:01\n"), nil, nil
	case name == "nmcli" && slices.Equal(args, []string{"-t", "-f", "GENERAL.CONNECTION", "device", "show", "eth0"}):
		connection := "old-office"
		if r.active {
			connection = "office"
		}
		return []byte("GENERAL.CONNECTION:" + connection + "\n"), nil, nil
	case name == "nmcli" && len(args) == 6 && args[0] == "-t" && args[2] == "connection.id,connection.type,connection.autoconnect,802-3-ethernet.mtu,ipv4.method,ipv6.method,ipv4.addresses,ipv6.addresses":
		return []byte("connection.id:office\nconnection.type:802-3-ethernet\n"), nil, nil
	case name == "nmcli" && slices.Equal(args, []string{"-t", "-f", "GENERAL.STATE,IP4.ADDRESS,IP6.ADDRESS", "device", "show", "eth0"}):
		state := "30 (disconnected)"
		if r.active {
			state = "100 (connected)"
		}
		return []byte("GENERAL.STATE:" + state + "\n"), nil, nil
	case name == "nmcli" && slices.Equal(args, []string{"-g", "GENERAL.DBUS-PATH", "device", "show", "eth0"}):
		return []byte("/org/freedesktop/NetworkManager/Devices/2\n"), nil, nil
	case name == "busctl" && len(args) > 4 && args[4] == "CheckpointCreate":
		return []byte("o \"" + r.checkpoint + "\"\n"), nil, nil
	case name == "busctl" && len(args) > 4 && args[4] == "CheckpointDestroy":
		return nil, nil, nil
	case name == "nmcli" && len(args) >= 3 && args[0] == "connection" && args[1] == "modify":
		return nil, nil, nil
	case name == "nmcli" && slices.Equal(args, []string{"connection", "up", "office", "ifname", "eth0"}):
		r.active = true
		return nil, nil, nil
	default:
		return nil, nil, fmt.Errorf("unexpected command: %s %v", name, args)
	}
}

func TestProfileApplyRestartAcknowledgementCleanupAndSecondCheck(t *testing.T) {
	audit, enforce := false, true
	now := time.Date(2026, 7, 17, 19, 0, 0, 0, time.UTC)
	runner := &acknowledgedConvergenceRunner{checkpoint: "/org/freedesktop/NetworkManager/Checkpoint/51"}
	provider := NewProfile(models.NetworkProfileResource{
		ResourceMeta: models.ResourceMeta{Enforce: &enforce},
		Name:         "uplink", Provider: models.NetworkProviderNetworkManager, Audit: &audit,
		Selector:    models.NetworkInterfaceSelector{Name: "eth0", Type: "ethernet"},
		ProfileName: "office", ProfileType: "ethernet", RollbackTimeout: "2m",
	}, runner)
	provider.StateDir = t.TempDir()
	provider.Now = func() time.Time { return now }
	provider.AfterFunc = func(time.Duration, func()) {}

	if first := provider.Check(context.Background()); first.Status != executor.Drifted {
		t.Fatalf("initial Check() = %+v", first)
	}
	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	store, err := networkstate.New(networkstate.Options{Root: provider.StateDir, Runner: runner, Now: provider.now})
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Status()
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement || status.Intent.Interface != "eth0" || status.Intent.Connection != "old-office" {
		t.Fatalf("restart status = %+v, %v", status, err)
	}
	status, err = store.Acknowledge(context.Background(), status.Intent.ID)
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseAcknowledged || !status.Intent.AuthenticatedAck {
		t.Fatalf("acknowledgement = %+v, %v", status, err)
	}
	store, err = networkstate.New(networkstate.Options{Root: provider.StateDir, Runner: runner, Now: provider.now})
	if err != nil {
		t.Fatal(err)
	}
	status, err = store.Status()
	if err != nil || status.Intent == nil || status.Intent.Phase != networkstate.PhaseAcknowledged || status.Intent.WatchdogArmed {
		t.Fatalf("clean restart status = %+v, %v", status, err)
	}
	if second := provider.Check(context.Background()); second.Status != executor.Compliant || second.ReasonCode != executor.ReasonCompliant {
		t.Fatalf("second Check() = %+v", second)
	} else if report, ok := second.Actual.(ProfileReport); !ok || !report.Acknowledged || report.RollbackOutcome != "acknowledged" {
		t.Fatalf("second Check() transaction report = %#v", second.Actual)
	}
}

func TestProfileEnforcementRequiresExplicitAuthorizationBeforeCheckpoint(t *testing.T) {
	audit := false
	runner := &executil.MockRunner{}
	provider := NewProfile(models.NetworkProfileResource{
		Name: "uplink", Provider: models.NetworkProviderNetworkManager, Audit: &audit,
		Selector: models.NetworkInterfaceSelector{Name: "eth0"}, ProfileName: "office",
		ProfileType: "ethernet", RollbackTimeout: "2m",
	}, runner)
	provider.StateDir = t.TempDir()

	err := provider.Preflight(context.Background())
	if err == nil || !strings.Contains(err.Error(), "explicit enforce authorization") {
		t.Fatalf("Preflight() error = %v, want explicit authorization rejection", err)
	}
	if len(runner.Calls) != 0 {
		t.Fatalf("unauthorized preflight reached NetworkManager: %+v", runner.Calls)
	}
	if provider.rollbackTimeout != 0*time.Second {
		t.Fatalf("unauthorized preflight armed timeout %s", provider.rollbackTimeout)
	}
}

func TestProfileEnforcementCreatesCheckpointBeforeActivation(t *testing.T) {
	audit, enforce, autoConnect := false, true, true
	checkpoint := "/org/freedesktop/NetworkManager/Checkpoint/9"
	deviceOutput := []byte("GENERAL.DEVICE:eth0\nGENERAL.TYPE:ethernet\nGENERAL.HWADDR:02:00:00:00:00:01\n")
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nmcli [-t -f GENERAL.DEVICE,GENERAL.TYPE,GENERAL.HWADDR device show]": {Stdout: deviceOutput},
		"nmcli [-t -f GENERAL.CONNECTION device show eth0]":                    {Stdout: []byte("GENERAL.CONNECTION:old-office\n")},
		"nmcli [-t -f connection.id,connection.type,connection.autoconnect,802-3-ethernet.mtu,ipv4.method,ipv6.method,ipv4.addresses,ipv6.addresses connection show office]": {
			Stdout: []byte("connection.id:office\nconnection.type:802-3-ethernet\nconnection.autoconnect:yes\nipv4.method:auto\nipv6.method:ignore\n"),
		},
		"nmcli [-t -f GENERAL.STATE,IP4.ADDRESS,IP6.ADDRESS device show eth0]": {Stdout: []byte("GENERAL.STATE:30 (disconnected)\n")},
		"nmcli [-g GENERAL.DBUS-PATH device show eth0]":                        {Stdout: []byte("/org/freedesktop/NetworkManager/Devices/2\n")},
		"busctl [call org.freedesktop.NetworkManager /org/freedesktop/NetworkManager org.freedesktop.NetworkManager CheckpointCreate aouu 1 /org/freedesktop/NetworkManager/Devices/2 120 0]": {Stdout: []byte("o \"" + checkpoint + "\"\n")},
		"nmcli [connection modify office connection.interface-name eth0 connection.autoconnect yes ipv4.method auto ipv6.method ignore]":                                                      {},
		"nmcli [connection up office ifname eth0]": {},
	}}
	provider := NewProfile(models.NetworkProfileResource{
		ResourceMeta: models.ResourceMeta{Enforce: &enforce},
		Name:         "uplink", Provider: models.NetworkProviderNetworkManager, Audit: &audit,
		Selector: models.NetworkInterfaceSelector{Name: "eth0"}, ProfileName: "office", ProfileType: "ethernet",
		AutoConnect: &autoConnect, IPv4Method: "auto", IPv6Method: "ignore", RollbackTimeout: "2m",
	}, runner)
	provider.StateDir = t.TempDir()
	provider.Now = func() time.Time { return time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC) }
	var watchdogDelay time.Duration
	provider.AfterFunc = func(delay time.Duration, _ func()) { watchdogDelay = delay }

	result := provider.ApplyResult(context.Background())
	if result.Status != executor.Changed || result.RollbackClass != executor.RollbackTransactional {
		t.Fatalf("ApplyResult() = %+v", result)
	}
	if watchdogDelay != 2*time.Minute {
		t.Fatalf("watchdog delay = %s", watchdogDelay)
	}
	checkpointCall, modifyCall, activateCall := -1, -1, -1
	for index, call := range runner.Calls {
		joined := call.Name + " " + strings.Join(call.Args, " ")
		switch {
		case strings.Contains(joined, "CheckpointCreate"):
			checkpointCall = index
		case strings.Contains(joined, "connection modify office"):
			modifyCall = index
		case strings.Contains(joined, "connection up office"):
			activateCall = index
		}
	}
	if checkpointCall < 0 || modifyCall <= checkpointCall || activateCall <= modifyCall {
		t.Fatalf("unsafe activation order: %+v", runner.Calls)
	}
	store, err := networkstate.New(networkstate.Options{Root: provider.StateDir, Runner: runner, Now: provider.now})
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Status()
	if err != nil || status.Intent == nil || status.Intent.Checkpoint != checkpoint || status.Intent.Phase != networkstate.PhaseAwaitingAcknowledgement || !strings.HasPrefix(status.Intent.PlanHash, "sha256:") {
		t.Fatalf("armed checkpoint status = %+v, err=%v", status, err)
	}
}

func TestProfileCheckRejectsAmbiguousInterfaceSelectorWithoutMutation(t *testing.T) {
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nmcli [-t -f GENERAL.DEVICE,GENERAL.TYPE,GENERAL.HWADDR device show]": {
			Stdout: []byte("GENERAL.DEVICE:eth0\nGENERAL.TYPE:ethernet\nGENERAL.HWADDR:02:00:00:00:00:01\n\nGENERAL.DEVICE:eth1\nGENERAL.TYPE:ethernet\nGENERAL.HWADDR:02:00:00:00:00:02\n"),
		},
	}}
	provider := NewProfile(models.NetworkProfileResource{
		Name: "uplink", Provider: models.NetworkProviderNetworkManager,
		Selector:    models.NetworkInterfaceSelector{Type: "ethernet"},
		ProfileName: "office", ProfileType: "ethernet",
	}, runner)
	check := provider.Check(context.Background())
	if check.Status != executor.CheckFailed || check.ReasonCode != "ambiguous_interface" || check.Err == nil || !strings.Contains(check.Err.Error(), "matched 2 interfaces") {
		t.Fatalf("ambiguous selector Check() = %+v", check)
	}
	if len(runner.Calls) != 1 {
		t.Fatalf("ambiguous selector continued toward mutation: %+v", runner.Calls)
	}
}

func TestProfileAuditReportsCredentialDriftWithoutSecretMaterial(t *testing.T) {
	const canary = "remotr-secret-canary-do-not-report"
	autoConnect := true
	runner := &executil.MockRunner{Next: map[string]executil.MockResult{
		"nmcli [-t -f GENERAL.DEVICE,GENERAL.TYPE,GENERAL.HWADDR device show]": {
			Stdout: []byte("GENERAL.DEVICE:wlan0\nGENERAL.TYPE:wifi\nGENERAL.HWADDR:02:00:00:00:00:0A\n"),
		},
		"nmcli [-t -f GENERAL.CONNECTION device show wlan0]": {Stdout: []byte("GENERAL.CONNECTION:office\n")},
		"nmcli [-t -f connection.id,connection.type,connection.autoconnect,802-11-wireless.mtu,ipv4.method,ipv6.method,ipv4.addresses,ipv6.addresses,802-11-wireless.ssid connection show office]": {
			Stdout: []byte("connection.id:office\nconnection.type:802-11-wireless\nconnection.autoconnect:yes\n802-11-wireless.mtu:1500\nipv4.method:auto\nipv6.method:ignore\n802-11-wireless.ssid:corp\n"),
		},
		"nmcli [-t connection show office]":                                     {Stdout: []byte("user.data:remotr.credential=sha256:old\n802-11-wireless-security.psk:" + canary + "\n")},
		"nmcli [-t -f GENERAL.STATE,IP4.ADDRESS,IP6.ADDRESS device show wlan0]": {Stdout: []byte("GENERAL.STATE:100 (connected)\n")},
	}}
	provider := NewProfile(models.NetworkProfileResource{
		Name: "wifi", Provider: models.NetworkProviderNetworkManager,
		Selector:    models.NetworkInterfaceSelector{Name: "wlan0", PermanentMAC: "02:00:00:00:00:0a", Type: "wifi"},
		ProfileName: "office", ProfileType: "wifi", AutoConnect: &autoConnect, MTU: 1500,
		IPv4Method: "auto", IPv6Method: "ignore", SSID: "corp", CredentialRef: "remotr:wifi/office@active",
	}, runner)
	check := provider.Check(context.Background())
	if check.Status != executor.Drifted || check.ReasonCode != "audit_plan" {
		t.Fatalf("credential-only drift Check() = %+v", check)
	}
	report, ok := check.Actual.(ProfileReport)
	if !ok || !report.Configured.Compliant || !report.Effective.Compliant || !report.CredentialDrift || report.CredentialReference != "remotr:wifi/office@active" || !strings.HasPrefix(report.CredentialFingerprint, "sha256:") {
		t.Fatalf("safe profile report = %#v", check.Actual)
	}
	if strings.Contains(fmt.Sprintf("%+v", check), canary) {
		t.Fatalf("profile report leaked credential canary: %+v", check)
	}
}
