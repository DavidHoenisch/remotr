package main

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
)

func TestReleaseApplicationOptions(t *testing.T) {
	app := newApplicationOptions()

	if app.Title != "Remotr Desktop" {
		t.Errorf("Title = %q, want Remotr Desktop", app.Title)
	}
	if app.Width != 1440 || app.Height != 900 {
		t.Errorf("initial size = %dx%d, want 1440x900", app.Width, app.Height)
	}
	if app.MinWidth != 1100 || app.MinHeight != 720 {
		t.Errorf("minimum size = %dx%d, want 1100x720", app.MinWidth, app.MinHeight)
	}
	if app.DisableResize {
		t.Error("desktop window must remain resizable")
	}
	if app.MaxWidth < 7680 || app.MaxHeight < 4320 {
		t.Errorf("maximum size = %dx%d, want room to maximize on an 8K desktop", app.MaxWidth, app.MaxHeight)
	}
	if app.AssetServer == nil || app.AssetServer.Assets == nil {
		t.Fatal("release assets are not embedded")
	}
	if _, err := fs.ReadFile(app.AssetServer.Assets, "frontend/dist/index.html"); err != nil {
		t.Fatalf("read embedded release index: %v", err)
	}
	if app.EnableDefaultContextMenu || app.Debug.OpenInspectorOnStartup {
		t.Error("release developer tools must remain disabled")
	}
	if app.BindingsAllowedOrigins != "" {
		t.Errorf("BindingsAllowedOrigins = %q, want embedded origin only", app.BindingsAllowedOrigins)
	}
	if app.Linux == nil || app.Linux.ProgramName != "remotr-desktop" {
		t.Fatalf("Linux application identity = %#v, want remotr-desktop", app.Linux)
	}
	if app.DragAndDrop == nil || !app.DragAndDrop.DisableWebViewDrop {
		t.Error("release webview must reject navigation by external file drop")
	}
}

func TestWailsMetadataIdentifiesRemotrDesktop(t *testing.T) {
	data, err := os.ReadFile("wails.json")
	if err != nil {
		t.Fatalf("read Wails metadata: %v", err)
	}
	var metadata struct {
		Version        string `json:"version"`
		Name           string `json:"name"`
		OutputFilename string `json:"outputfilename"`
		Info           struct {
			CompanyName    string `json:"companyName"`
			ProductName    string `json:"productName"`
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	if err := json.Unmarshal(data, &metadata); err != nil {
		t.Fatalf("parse Wails metadata: %v", err)
	}
	if metadata.Version != "2" || metadata.Name != "remotr-desktop" || metadata.OutputFilename != "remotr-desktop" {
		t.Errorf("Wails identity = version %q, name %q, output %q", metadata.Version, metadata.Name, metadata.OutputFilename)
	}
	if metadata.Info.CompanyName != "Remotr" || metadata.Info.ProductName != "Remotr Desktop" || metadata.Info.ProductVersion != "0.0.0" {
		t.Errorf("product metadata = %#v, want Remotr Desktop development identity", metadata.Info)
	}
}

func TestFrontendDevWatcherUsesExplicitLoopbackURL(t *testing.T) {
	wailsData, err := os.ReadFile("wails.json")
	if err != nil {
		t.Fatalf("read Wails metadata: %v", err)
	}
	var wailsMetadata struct {
		DevWatcher   string `json:"frontend:dev:watcher"`
		DevServerURL string `json:"frontend:dev:serverUrl"`
	}
	if err := json.Unmarshal(wailsData, &wailsMetadata); err != nil {
		t.Fatalf("parse Wails metadata: %v", err)
	}
	const wantURL = "http://127.0.0.1:5173"
	if wailsMetadata.DevServerURL != wantURL {
		t.Errorf("Wails frontend dev server URL = %q, want explicit %q", wailsMetadata.DevServerURL, wantURL)
	}
	const wantWatcher = "./node_modules/.bin/vite --host 127.0.0.1 --port 5173 --strictPort"
	if wailsMetadata.DevWatcher != wantWatcher {
		t.Errorf("Wails frontend dev watcher = %q, want direct local Vite command %q", wailsMetadata.DevWatcher, wantWatcher)
	}

	packageData, err := os.ReadFile("frontend/package.json")
	if err != nil {
		t.Fatalf("read frontend package metadata: %v", err)
	}
	var packageMetadata struct {
		Scripts map[string]string `json:"scripts"`
	}
	if err := json.Unmarshal(packageData, &packageMetadata); err != nil {
		t.Fatalf("parse frontend package metadata: %v", err)
	}
	const wantCommand = "vite --host 127.0.0.1 --port 5173 --strictPort"
	if got := packageMetadata.Scripts["dev"]; got != wantCommand {
		t.Errorf("frontend dev script = %q, want %q", got, wantCommand)
	}
}

func TestReleaseAssetPolicyRejectsRemoteContent(t *testing.T) {
	app := newApplicationOptions()
	if app.AssetServer == nil || app.AssetServer.Middleware == nil {
		t.Fatal("release asset middleware is not configured")
	}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := app.AssetServer.Middleware(next)

	tests := []struct {
		name       string
		target     string
		wantStatus int
	}{
		{name: "embedded relative asset", target: "/index.html", wantStatus: http.StatusNoContent},
		{name: "embedded Wails asset", target: "wails://wails/index.html", wantStatus: http.StatusNoContent},
		{name: "remote HTTPS content", target: "https://example.invalid/app", wantStatus: http.StatusForbidden},
		{name: "local file content", target: "file:///tmp/untrusted.html", wantStatus: http.StatusForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target, err := url.Parse(test.target)
			if err != nil {
				t.Fatalf("parse test target: %v", err)
			}
			request := &http.Request{Method: http.MethodGet, URL: target}
			response := httptest.NewRecorder()

			handler.ServeHTTP(response, request)

			if response.Code != test.wantStatus {
				t.Errorf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if test.wantStatus == http.StatusNoContent {
				policy := response.Header().Get("Content-Security-Policy")
				if !strings.Contains(policy, "default-src 'self'") || !strings.Contains(policy, "frame-ancestors 'none'") {
					t.Errorf("Content-Security-Policy = %q, want embedded-only policy", policy)
				}
				if strings.Contains(policy, "'unsafe-inline'") {
					t.Errorf("release Content-Security-Policy = %q, must not allow inline content", policy)
				}
			}
		})
	}
}
