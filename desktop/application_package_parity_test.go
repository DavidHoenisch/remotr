package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/apppackages"
)

const applicationPackageCredentialCanary = "application-package-private-key-canary"

func TestApplicationPackageNativeWorkflowBuildsAndValidatesWithoutGitOrServerMutation(t *testing.T) {
	root := t.TempDir()
	parent := filepath.Join(root, "packages")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	gitIndex := filepath.Join(root, ".git", "index")
	if err := os.MkdirAll(filepath.Dir(gitIndex), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gitIndex, []byte("unchanged-index"), 0o600); err != nil {
		t.Fatal(err)
	}

	var selectedSource string
	var selectedArchive string
	output := filepath.Join(root, "dist", "internal_tool-1.2.3.zip")
	service := NewApplicationPackageService(ApplicationPackageDialogs{
		ChooseCreateParent: func(context.Context) (string, error) { return parent, nil },
		ChooseSource:       func(context.Context) (string, error) { return selectedSource, nil },
		ChooseArchive:      func(context.Context) (string, error) { return selectedArchive, nil },
		ChooseBuildOutput: func(_ context.Context, suggestedName string) (string, error) {
			if suggestedName != "internal_tool-1.2.3.zip" {
				t.Fatalf("suggested output = %q", suggestedName)
			}
			return output, nil
		},
	})
	app := NewApp("test", WithApplicationPackageService(service))

	created, err := app.CreateLocalPackage(LocalPackageCreateRequest{
		DirectoryName: "tool",
		Name:          "internal/tool",
		Version:       "1.2.3",
		Mode:          "binary",
	})
	if err != nil {
		t.Fatalf("create local package: %v", err)
	}
	if created.Name != "internal/tool" || created.Version != "1.2.3" || created.Mode != "binary" || created.LocationName != "tool" {
		t.Fatalf("created package = %#v", created)
	}
	selectedSource = filepath.Join(parent, "tool")

	source, err := app.ChooseLocalPackageSource()
	if err != nil {
		t.Fatalf("choose local package source: %v", err)
	}
	if source.Name != "internal/tool" || source.Version != "1.2.3" || source.LocationName != "tool" {
		t.Fatalf("source package = %#v", source)
	}

	built, err := app.BuildLocalPackage()
	if err != nil {
		t.Fatalf("build local package: %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read built archive: %v", err)
	}
	wantDigest := sha256.Sum256(data)
	if built.Name != "internal/tool" || built.Version != "1.2.3" || built.SHA256 != hex.EncodeToString(wantDigest[:]) || built.SizeBytes != int64(len(data)) || built.FileName != filepath.Base(output) {
		t.Fatalf("built package = %#v", built)
	}
	if _, err := apppackages.ValidateZip(data); err != nil {
		t.Fatalf("built archive is invalid: %v", err)
	}

	selectedArchive = output
	validated, err := app.ChooseAppPackageArchive()
	if err != nil {
		t.Fatalf("validate selected archive: %v", err)
	}
	if validated.Name != built.Name || validated.Version != built.Version || validated.Mode != built.Mode || validated.SHA256 != built.SHA256 || validated.SizeBytes != built.SizeBytes || validated.FileName != built.FileName || validated.Source != "selected" {
		t.Fatalf("validated package = %#v, want built integrity metadata with selected source", validated)
	}

	indexAfter, err := os.ReadFile(gitIndex)
	if err != nil {
		t.Fatal(err)
	}
	if string(indexAfter) != "unchanged-index" {
		t.Fatalf("Git index changed: %q", indexAfter)
	}
}

type applicationPackageServerState struct {
	mu            sync.Mutex
	uploadBodies  [][]byte
	uploadQueries []string
	deleteQueries []string
	forbid        bool
	published     bool
}

func TestApplicationPackageCatalogParityUsesAuthenticatedBytesAndExactDeletionScope(t *testing.T) {
	archivePath, archiveData, archiveSummary := buildApplicationPackageFixture(t)
	var logOutput bytes.Buffer
	originalLogOutput := log.Writer()
	log.SetOutput(&logOutput)
	t.Cleanup(func() { log.SetOutput(originalLogOutput) })

	app, state, settingsDir := newApplicationPackageCatalogTestApp(t, archivePath)
	selected, err := app.ChooseAppPackageArchive()
	if err != nil {
		t.Fatalf("choose archive: %v", err)
	}
	if selected.SHA256 != archiveSummary.SHA256 || selected.SizeBytes != archiveSummary.Size {
		t.Fatalf("selected archive = %#v", selected)
	}

	listed, err := app.ListAppPackages("")
	if err != nil {
		t.Fatalf("list packages: %v", err)
	}
	if len(listed) != 1 || listed[0].Name != "internal/existing" || listed[0].Version != "9.0.0" {
		t.Fatalf("listed packages = %#v", listed)
	}
	shown, err := app.LoadAppPackage("internal/existing", "9.0.0")
	if err != nil {
		t.Fatalf("show package: %v", err)
	}
	if shown.ID != "package-existing" || shown.InstallMode != "binary" || shown.SHA256 != strings.Repeat("a", 64) {
		t.Fatalf("shown package = %#v", shown)
	}

	badPublish := AppPackagePublishRequest{
		Name: archiveSummary.Manifest.Name, Version: archiveSummary.Manifest.Version,
		SHA256: archiveSummary.SHA256, Confirmation: strings.ToUpper(archiveSummary.Manifest.Name + "@" + archiveSummary.Manifest.Version),
	}
	if _, err := app.PublishAppPackage(badPublish); err == nil {
		t.Fatal("case-insensitive publish confirmation succeeded")
	}
	state.mu.Lock()
	if len(state.uploadBodies) != 0 {
		t.Fatalf("invalid publish reached server: %d requests", len(state.uploadBodies))
	}
	state.mu.Unlock()

	if err := os.WriteFile(archivePath, append(slices.Clone(archiveData), byte('x')), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.PublishAppPackage(AppPackagePublishRequest{
		Name: archiveSummary.Manifest.Name, Version: archiveSummary.Manifest.Version,
		SHA256: archiveSummary.SHA256, Confirmation: archiveSummary.Manifest.Name + "@" + archiveSummary.Manifest.Version,
	}); err == nil {
		t.Fatal("archive changed after validation was published")
	}
	state.mu.Lock()
	if len(state.uploadBodies) != 0 {
		t.Fatalf("changed archive reached server: %d requests", len(state.uploadBodies))
	}
	state.mu.Unlock()
	if err := os.WriteFile(archivePath, archiveData, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := app.ChooseAppPackageArchive(); err != nil {
		t.Fatalf("revalidate restored archive: %v", err)
	}

	published, err := app.PublishAppPackage(AppPackagePublishRequest{
		Name: archiveSummary.Manifest.Name, Version: archiveSummary.Manifest.Version,
		SHA256: archiveSummary.SHA256, Confirmation: archiveSummary.Manifest.Name + "@" + archiveSummary.Manifest.Version,
	})
	if err != nil {
		t.Fatalf("publish package: %v", err)
	}
	if published.Name != archiveSummary.Manifest.Name || published.Version != archiveSummary.Manifest.Version || published.SHA256 != archiveSummary.SHA256 {
		t.Fatalf("published package = %#v", published)
	}
	state.mu.Lock()
	uploadBodies := slices.Clone(state.uploadBodies)
	uploadQueries := slices.Clone(state.uploadQueries)
	state.mu.Unlock()
	if len(uploadBodies) != 1 || !bytes.Equal(uploadBodies[0], archiveData) || !slices.Equal(uploadQueries, []string{""}) {
		t.Fatalf("upload request bodies=%d queries=%v", len(uploadBodies), uploadQueries)
	}

	if _, err := app.DeleteAppPackage(AppPackageDeleteRequest{
		Name: "internal/existing", Version: "9.0.0", DeleteObject: true,
		Confirmation: "internal/existing@9.0.0 CATALOG ONLY",
	}); err == nil {
		t.Fatal("mismatched object deletion confirmation succeeded")
	}
	state.mu.Lock()
	if len(state.deleteQueries) != 0 {
		t.Fatalf("invalid delete reached server: %v", state.deleteQueries)
	}
	state.mu.Unlock()

	deleted, err := app.DeleteAppPackage(AppPackageDeleteRequest{
		Name: "internal/existing", Version: "9.0.0", DeleteObject: true,
		Confirmation: "internal/existing@9.0.0 DELETE OBJECT",
	})
	if err != nil {
		t.Fatalf("delete package and object: %v", err)
	}
	if deleted.Name != "internal/existing" || deleted.Version != "9.0.0" || deleted.Scope != "catalog_and_object" {
		t.Fatalf("delete result = %#v", deleted)
	}
	state.mu.Lock()
	deleteQueries := slices.Clone(state.deleteQueries)
	state.mu.Unlock()
	if !slices.Equal(deleteQueries, []string{"name=internal%2Fexisting&version=9.0.0&delete_object=true"}) {
		t.Fatalf("delete queries = %v", deleteQueries)
	}

	state.mu.Lock()
	state.forbid = true
	state.mu.Unlock()
	_, err = app.ListAppPackages("")
	var forbidden *ActionFailure
	if !errors.As(err, &forbidden) || forbidden.Kind != ActionForbidden {
		t.Fatalf("forbidden list error = %T %v", err, err)
	}

	views, err := json.Marshal([]any{selected, listed, shown, published, deleted})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(views, archiveData) || bytes.Contains(views, []byte(applicationPackageCredentialCanary)) {
		t.Fatalf("ordinary package views disclosed protected bytes: %s", views)
	}
	if strings.Contains(logOutput.String(), applicationPackageCredentialCanary) {
		t.Fatalf("logs disclosed credential canary: %s", logOutput.String())
	}
	assertPathsExcludeCanary(t, applicationPackageCredentialCanary, settingsDir)
}

func TestApplicationPackageNativeSelectionRejectsUnsafePathsAndSources(t *testing.T) {
	dialogCalled := false
	service := NewApplicationPackageService(ApplicationPackageDialogs{
		ChooseCreateParent: func(context.Context) (string, error) {
			dialogCalled = true
			return t.TempDir(), nil
		},
	})
	if _, err := service.CreateLocal(t.Context(), LocalPackageCreateRequest{
		DirectoryName: "../escape", Name: "internal/tool", Version: "1.0.0", Mode: "binary",
	}); err == nil {
		t.Fatal("unsafe package directory name succeeded")
	}
	if dialogCalled {
		t.Fatal("unsafe directory name reached native dialog")
	}

	archivePath, _, _ := buildApplicationPackageFixture(t)
	symlink := filepath.Join(t.TempDir(), "selected.zip")
	if err := os.Symlink(archivePath, symlink); err != nil {
		t.Fatal(err)
	}
	service = NewApplicationPackageService(ApplicationPackageDialogs{
		ChooseArchive: func(context.Context) (string, error) { return symlink, nil },
	})
	if _, err := service.ChooseArchive(t.Context()); err == nil {
		t.Fatal("symbolic-link archive succeeded")
	}

	source := filepath.Join(t.TempDir(), "source")
	if _, err := apppackages.CreateScaffold(apppackages.ScaffoldOptions{
		Dir: source, Name: "internal/tool", Version: "1.0.0", Mode: "binary",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(archivePath, filepath.Join(source, "linked-payload")); err != nil {
		t.Fatal(err)
	}
	service = NewApplicationPackageService(ApplicationPackageDialogs{
		ChooseSource: func(context.Context) (string, error) { return source, nil },
	})
	if _, err := service.ChooseSource(t.Context()); err == nil {
		t.Fatal("package source containing a symbolic link succeeded")
	}
}

func FuzzLocalPackageDirectoryName(f *testing.F) {
	for _, seed := range []string{"tool", "internal-tool", "../escape", "a/b", "", "."} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input string) {
		name, err := validateLocalPackageDirectoryName(input)
		if err != nil {
			return
		}
		if name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, `/\\`) || len(name) > 128 {
			t.Fatalf("accepted unsafe directory name %q as %q", input, name)
		}
	})
}

func buildApplicationPackageFixture(t *testing.T) (string, []byte, apppackages.ZipSummary) {
	t.Helper()
	directory := filepath.Join(t.TempDir(), "source")
	if _, err := apppackages.CreateScaffold(apppackages.ScaffoldOptions{
		Dir: directory, Name: "internal/tool", Version: "1.2.3", Mode: "binary",
	}); err != nil {
		t.Fatal(err)
	}
	data, summary, err := apppackages.BuildZipFromDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "internal_tool-1.2.3.zip")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, data, summary
}

func newApplicationPackageCatalogTestApp(t *testing.T, archivePath string) (*App, *applicationPackageServerState, string) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	state := &applicationPackageServerState{}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		state.mu.Lock()
		forbidden := state.forbid
		state.mu.Unlock()
		if forbidden && request.URL.Path != "/v1/admin/me" {
			http.Error(response, applicationPackageCredentialCanary, http.StatusForbidden)
			return
		}
		switch {
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/me":
			_, _ = response.Write([]byte(`{"operator_id":"operator-packages","roles":["package_manager"]}`))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/app-packages":
			_, _ = response.Write([]byte("[" + applicationPackageRecordJSON("package-existing", "internal/existing", "9.0.0", strings.Repeat("a", 64)) + "]"))
		case request.Method == http.MethodGet && request.URL.Path == "/v1/admin/app-packages/detail":
			_, _ = response.Write([]byte(applicationPackageRecordJSON("package-existing", request.URL.Query().Get("name"), request.URL.Query().Get("version"), strings.Repeat("a", 64))))
		case request.Method == http.MethodPost && request.URL.Path == "/v1/admin/app-packages/upload":
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Errorf("read upload: %v", err)
				return
			}
			summary, err := apppackages.ValidateZip(body)
			if err != nil {
				t.Errorf("validate uploaded package: %v", err)
				http.Error(response, "invalid archive", http.StatusBadRequest)
				return
			}
			state.mu.Lock()
			state.uploadBodies = append(state.uploadBodies, slices.Clone(body))
			state.uploadQueries = append(state.uploadQueries, request.URL.RawQuery)
			state.published = true
			state.mu.Unlock()
			_, _ = response.Write([]byte(applicationPackageRecordJSON("package-published", summary.Manifest.Name, summary.Manifest.Version, summary.SHA256)))
		case request.Method == http.MethodDelete && request.URL.Path == "/v1/admin/app-packages/detail":
			state.mu.Lock()
			state.deleteQueries = append(state.deleteQueries, request.URL.RawQuery)
			state.mu.Unlock()
			response.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(response, request)
		}
	}))
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{fixture.serverCert}, ClientAuth: tls.RequireAndVerifyClientCert,
		ClientCAs: connectionCertPool(t, fixture.caPEM), MinVersion: tls.VersionTLS12,
		Time: func() time.Time { return connectionTestTime },
	}
	server.StartTLS()
	t.Cleanup(server.Close)

	stateDir := fixture.saveClientState(t, "operator-packages", connectionTestTime.Add(-time.Hour), connectionTestTime.Add(time.Hour), fixture.caPEM)
	if err := os.WriteFile(filepath.Join(stateDir, "package-canary.txt"), []byte(applicationPackageCredentialCanary), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect package Operator: %v", err)
	}
	service := NewApplicationPackageService(ApplicationPackageDialogs{
		ChooseArchive: func(context.Context) (string, error) { return archivePath, nil },
	})
	app := NewApp("test", WithApplicationPackageService(service))
	app.sessions = manager
	settingsDir := t.TempDir()
	app.profiles = NewProfileService(filepath.Join(settingsDir, "desktop-profiles.json"), filepath.Join(settingsDir, "operator-config.yaml"))
	return app, state, settingsDir
}

func applicationPackageRecordJSON(id, name, version, digest string) string {
	return `{"id":"` + id + `","name":"` + name + `","version":"` + version + `","s3_key":"app-packages/` + name + `/` + version + `/archive.zip","sha256":"` + digest + `","manifest":{"schemaVersion":1,"name":"` + name + `","version":"` + version + `","install":{"mode":"binary","files":[{"src":"bin/tool","dest":"/usr/local/bin/tool"}]}},"created_at":"2032-03-04T05:05:07Z"}`
}
