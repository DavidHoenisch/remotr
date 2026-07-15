package networkmanager

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/executil"
	"github.com/DavidHoenisch/remotr/internal/executor"
	"github.com/DavidHoenisch/remotr/internal/models"
)

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
		"nmcli [-t -f connection.id,connection.type,connection.autoconnect,802-3-ethernet.mtu,802-11-wireless.mtu,ipv4.method,ipv6.method,ipv4.addresses,ipv6.addresses,802-11-wireless.ssid,user.data connection show office]": {
			Stdout: []byte("connection.id:office\nconnection.type:802-11-wireless\nconnection.autoconnect:yes\n802-11-wireless.mtu:1500\nipv4.method:auto\nipv6.method:ignore\n802-11-wireless.ssid:corp\nuser.data:remotr.credential=sha256:old\n802-11-wireless-security.psk:" + canary + "\n"),
		},
		"nmcli [-t -f GENERAL.STATE,IP4.ADDRESS,IP6.ADDRESS device show wlan0]": {Stdout: []byte("GENERAL.STATE:100 (connected)\n")},
	}}
	provider := NewProfile(models.NetworkProfileResource{
		Name: "wifi", Provider: models.NetworkProviderNetworkManager,
		Selector:    models.NetworkInterfaceSelector{Name: "wlan0", PermanentMAC: "02:00:00:00:00:0a", Type: "wifi"},
		ProfileName: "office", ProfileType: "wifi", AutoConnect: &autoConnect, MTU: 1500,
		IPv4Method: "auto", IPv6Method: "ignore", SSID: "corp", CredentialRef: "remotr:wifi/office",
	}, runner)
	check := provider.Check(context.Background())
	if check.Status != executor.Drifted || check.ReasonCode != "audit_plan" {
		t.Fatalf("credential-only drift Check() = %+v", check)
	}
	report, ok := check.Actual.(ProfileReport)
	if !ok || !report.Configured.Compliant || !report.Effective.Compliant || !report.CredentialDrift || report.CredentialReference != "remotr:wifi/office" || !strings.HasPrefix(report.CredentialFingerprint, "sha256:") {
		t.Fatalf("safe profile report = %#v", check.Actual)
	}
	if strings.Contains(fmt.Sprintf("%+v", check), canary) {
		t.Fatalf("profile report leaked credential canary: %+v", check)
	}
}
