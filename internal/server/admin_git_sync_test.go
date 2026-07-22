package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/DavidHoenisch/remotr/internal/configrepo"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/providermatrix"
	"github.com/DavidHoenisch/remotr/internal/types"
)

// OS-AEC-104, OS-AEC-105, and OS-AEC-107. Public seam: authenticated Admin
// Git-sync API. It exposes the same stable diagnostic identity as the public
// configuration CLI without advancing release state.
func TestAdminGitSyncReturnsProviderValidationDiagnostic(t *testing.T) {
	defaultMatrix, err := providermatrix.Default()
	if err != nil {
		t.Fatal(err)
	}
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "11111111-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))
	base := models.Configuration{
		Name: "apt-applications", TargetDistros: []types.Distro{types.Debian}, TargetArch: []types.Architecture{types.X86},
		Packages: []models.Package{{Name: "curl", Present: true, PM: types.Apt}},
	}
	tests := []struct {
		name   string
		mutate func(*models.Configuration)
		matrix providermatrix.Matrix
		code   string
	}{
		{name: "missing distro", mutate: func(configuration *models.Configuration) { configuration.TargetDistros = nil }, matrix: defaultMatrix, code: configrepo.ProviderReleaseTargetDistrosCode},
		{name: "missing architecture", mutate: func(configuration *models.Configuration) { configuration.TargetArch = nil }, matrix: defaultMatrix, code: configrepo.ProviderReleaseTargetArchCode},
		{name: "unsupported provider row", mutate: func(*models.Configuration) {}, matrix: providermatrix.Matrix{Version: 1}, code: configrepo.ProviderReleaseEvidenceCode},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			configuration := base
			test.mutate(&configuration)
			validationErr := configrepo.ValidateProviderRelease(models.State{SchemaVersion: 1, Configurations: []models.Configuration{configuration}}, test.matrix)
			if validationErr == nil {
				t.Fatal("fixture did not produce provider release validation error")
			}
			srv := New(Config{
				Admin: admin, CACert: caCert, CAKey: caKey, CACertPEM: caPEM,
				GitSync: func(context.Context) error { return validationErr },
			})
			req := httptest.NewRequest(http.MethodPost, "/v1/admin/git-sync", nil)
			req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
			rec := httptest.NewRecorder()
			srv.Handler().ServeHTTP(rec, req)

			if rec.Code != http.StatusUnprocessableEntity ||
				!strings.Contains(rec.Body.String(), test.code) ||
				!strings.Contains(rec.Body.String(), "apt-applications") {
				t.Fatalf("status=%d body=%q", rec.Code, rec.Body.String())
			}
		})
	}
}

// OS-AEC-106. Unknown Git-sync failures are correlated without reflecting
// caller-controlled credentials into the Admin response or server log.
func TestAdminGitSyncRedactsUnclassifiedFailureDetails(t *testing.T) {
	const canary = "git-sync-secret-canary"
	caCert, caKey, caPEM := testCAForEnroll(t)
	admin := newMockAdmin()
	opCred, err := pki.IssueOperatorCredential(caCert, caKey, "22222222-2222-3333-4444-555555555555")
	if err != nil {
		t.Fatal(err)
	}
	_ = admin.RegisterOperatorCredential(identity.Fingerprint(opCred.Cert))
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	srv := New(Config{
		Admin: admin, CACert: caCert, CAKey: caKey, CACertPEM: caPEM,
		GitSync: func(context.Context) error {
			return fmt.Errorf("clone https://operator:%s@example.test/repository failed", canary)
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/git-sync", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCred.Cert}}
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError || strings.Contains(rec.Body.String(), canary) || strings.Contains(logs.String(), canary) {
		t.Fatalf("status=%d body=%q logs=%q", rec.Code, rec.Body.String(), logs.String())
	}
	if !strings.Contains(logs.String(), "request_id=") || !strings.Contains(logs.String(), "error_type=") {
		t.Fatalf("redacted log omitted correlation and failure class: %q", logs.String())
	}
}
