package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/apppackages"
)

type memCatalog struct {
	rec apppackages.PackageRecord
}

func (m *memCatalog) Create(_ context.Context, rec apppackages.PackageRecord) (apppackages.PackageRecord, error) {
	rec.ID = "pkg-1"
	rec.CreatedAt = time.Now().UTC()
	m.rec = rec
	return rec, nil
}

func (m *memCatalog) Get(_ context.Context, name, version string) (apppackages.PackageRecord, error) {
	if m.rec.Name == name && m.rec.Version == version {
		return m.rec, nil
	}
	return apppackages.PackageRecord{}, apppackages.ErrNotFound
}

func (m *memCatalog) List(_ context.Context, _ string) ([]apppackages.PackageRecord, error) {
	if m.rec.Name == "" {
		return nil, nil
	}
	return []apppackages.PackageRecord{m.rec}, nil
}

func (m *memCatalog) Delete(_ context.Context, name, version string) error {
	if m.rec.Name == name && m.rec.Version == version {
		m.rec = apppackages.PackageRecord{}
		return nil
	}
	return apppackages.ErrNotFound
}

type fakeURLService struct{}

func (fakeURLService) DownloadURL(_ context.Context, _, _ string) (apppackages.DownloadURL, error) {
	return apppackages.DownloadURL{
		URL:       "https://example.com/pkg.zip",
		SHA256:    "abc",
		ExpiresAt: time.Now().UTC().Add(time.Hour),
	}, nil
}

func TestHandleCreateAppPackage(t *testing.T) {
	catalog := &memCatalog{}
	s := New(Config{AppPackages: catalog})
	body, _ := json.Marshal(createAppPackageRequest{
		Name:    "demo/tool",
		Version: "1.0.0",
		S3Key:   "app-packages/demo/tool/1.0.0/demo-tool-1.0.0.zip",
		SHA256:  "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Manifest: apppackages.Manifest{
			SchemaVersion: 1,
			Name:          "demo/tool",
			Version:       "1.0.0",
			Install: apppackages.InstallSpec{
				Mode: "binary",
				Files: []apppackages.InstallFile{{
					Src:  "bin/tool",
					Dest: "/usr/local/bin/tool",
				}},
			},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/admin/app-packages", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	s.handleCreateAppPackage(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHandleAppPackageDownloadURL(t *testing.T) {
	catalog := &memCatalog{rec: apppackages.PackageRecord{
		Name: "demo/tool", Version: "1.0.0", S3Key: "k", SHA256: "abc",
	}}
	s := New(Config{AppPackages: catalog, AppPackageURLs: fakeURLService{}})
	body := []byte(`{"name":"demo/tool","version":"1.0.0"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/app-packages/download-url", bytes.NewReader(body))
	uri, err := url.Parse("urn:remotr:endpoint:11111111-1111-1111-1111-111111111111")
	if err != nil {
		t.Fatal(err)
	}
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	s.handleAppPackageDownloadURL(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}
