package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	opcreds "github.com/DavidHoenisch/remotr/internal/operator/credentials"
)

const setupCredentialCanary = "setup-private-credential-canary-never-view"

func TestSetupMaintenanceParityUsesProtectedProfilesClassifiedDoctorAndTruthfulLinuxUpdates(t *testing.T) {
	fixture := newConnectionTLSFixture(t)
	var identityRequests atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/v1/admin/me" {
			http.NotFound(response, request)
			return
		}
		identityRequests.Add(1)
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusForbidden)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write([]byte(`{"operator_id":"operator-setup","roles":["global_admin"]}`))
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{fixture.serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    connectionCertPool(t, fixture.caPEM),
		MinVersion:   tls.VersionTLS12,
		Time:         func() time.Time { return connectionTestTime },
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	stateDir := fixture.saveClientState(t, "operator-setup", connectionTestTime.Add(-time.Hour), connectionTestTime.Add(time.Hour), fixture.caPEM)
	credentialLayout, err := opcreds.Layout(stateDir)
	if err != nil {
		t.Fatal(err)
	}
	privateKey, err := os.ReadFile(credentialLayout.Key)
	if err != nil {
		t.Fatal(err)
	}
	certificate, err := os.ReadFile(credentialLayout.Cert)
	if err != nil {
		t.Fatal(err)
	}
	settingsDir := t.TempDir()
	profilesPath := filepath.Join(settingsDir, "desktop-profiles.json")
	configPath := filepath.Join(settingsDir, "operator-config.yaml")
	if err := os.WriteFile(configPath, []byte("server_url: "+server.URL+"\nstate_dir: "+stateDir+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	opened := []string{}
	service := NewSetupMaintenanceService(SetupMaintenanceOptions{
		ApplicationVersion:  "v9.0.0",
		DesktopProfilesPath: profilesPath,
		GOARCH:              "amd64",
		GOOS:                "linux",
		OperatorConfigPath:  configPath,
		ReleaseCheck: func(context.Context) (string, error) {
			return "v9.1.0", nil
		},
	})
	app := NewApp("v9.0.0", WithSetupMaintenanceService(service), WithExternalLinkOpener(func(_ context.Context, target string) error {
		opened = append(opened, target)
		return nil
	}))
	app.profiles = NewProfileService(profilesPath, configPath)

	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	if err := app.SaveProfile(profile); err != nil {
		t.Fatalf("save profile: %v", err)
	}
	setup, err := app.LoadSetupMaintenance()
	if err != nil {
		t.Fatalf("load setup maintenance: %v", err)
	}
	if setup.Application.Name != "Remotr Desktop" || setup.Application.Version != "v9.0.0" || setup.Application.Platform != "linux" || setup.Application.Architecture != "amd64" {
		t.Fatalf("application setup metadata = %#v", setup.Application)
	}
	if setup.StandardConfigPath != configPath || setup.DesktopProfilesPath != profilesPath || len(setup.Profiles) != 1 || setup.Profiles[0] != profile {
		t.Fatalf("setup view = %#v", setup)
	}

	report, err := app.RunDesktopDoctor(profile)
	if err != nil {
		t.Fatalf("run desktop doctor: %v", err)
	}
	if !report.Healthy || report.OperatorID != "operator-setup" || !slices.Equal(report.Roles, []string{"global_admin"}) || len(report.Checks) != 5 {
		t.Fatalf("doctor report = %#v", report)
	}
	if got := identityRequests.Load(); got != 1 {
		t.Fatalf("identity requests = %d, want 1", got)
	}

	invalid := profile
	invalid.ServerURL = "http://insecure.example"
	failed, err := app.RunDesktopDoctor(invalid)
	if err != nil {
		t.Fatalf("run invalid doctor: %v", err)
	}
	if failed.Healthy || failed.Checks[0].Status != "fail" || identityRequests.Load() != 1 {
		t.Fatalf("invalid doctor report = %#v; requests = %d", failed, identityRequests.Load())
	}

	if err := app.OpenRemotrDocumentation(); err != nil {
		t.Fatalf("open documentation: %v", err)
	}
	if !slices.Equal(opened, []string{"https://davidhoenisch.github.io/remotr"}) {
		t.Fatalf("documentation handoffs = %v", opened)
	}
	update, err := app.CheckDesktopUpdate()
	if err != nil {
		t.Fatalf("check desktop update: %v", err)
	}
	if update.CurrentVersion != "v9.0.0" || update.LatestVersion != "v9.1.0" || !update.UpdateAvailable || update.InstallSupported || update.Platform != "linux/amd64" {
		t.Fatalf("desktop update = %#v", update)
	}

	encoded, err := os.ReadFile(profilesPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte(setupCredentialCanary)) || bytes.Contains(encoded, privateKey) || bytes.Contains(encoded, certificate) {
		t.Fatalf("desktop profile settings disclosed credential material: %s", encoded)
	}
	if info, err := os.Stat(profilesPath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("desktop profile permissions = %v, %v", info, err)
	}
}
