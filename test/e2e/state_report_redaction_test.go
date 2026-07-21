//go:build e2e

package e2e

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentdiagnostics "github.com/DavidHoenisch/remotr/internal/agent/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/agent/engine"
	"github.com/DavidHoenisch/remotr/internal/agent/pipeline"
	agentsync "github.com/DavidHoenisch/remotr/internal/agent/sync"
	diagcatalog "github.com/DavidHoenisch/remotr/internal/diagnostics"
	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/rollbackstore"
	"github.com/DavidHoenisch/remotr/internal/secrets"
	"github.com/DavidHoenisch/remotr/internal/server"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
	"github.com/DavidHoenisch/remotr/test/testsupport"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OS-AEC-084: one secret canary crosses the composed agent, authenticated
// Sync, durable Postgres, reconstructed server, Admin API and CLI, diagnostic
// collection, database restore validation, and endpoint rollback recovery
// seams without entering an unsafe artifact. Protected payload recovery is
// transient, and both server and endpoint persistence are cleaned afterward.
func TestSecretCanaryEndToEndAcrossPersistenceRestartAndRecovery(t *testing.T) {
	ctx := testsupport.Context(t, 45*time.Second)
	databaseURL := envOr("REMOTR_E2E_DATABASE_URL", "postgres://remotr:remotr@127.0.0.1:5432/remotr?sslmode=disable")
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Postgres unavailable: %v", err)
	}

	canary := testsupport.SecretCanary("end-to-end-restart-recovery")
	fleet := "canary-e2e"
	endpointID := uuid.NewString()
	secretName := "e2e/canary-" + strings.ReplaceAll(uuid.NewString(), "-", "")
	operatorFingerprint := ""
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM endpoints WHERE id = $1`, endpointID)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM secret_versions WHERE name = $1`, secretName)
		_, _ = pool.Exec(cleanupCtx, `DELETE FROM secret_names WHERE name = $1`, secretName)
		if operatorFingerprint != "" {
			_, _ = pool.Exec(cleanupCtx, `DELETE FROM operator_credentials WHERE cert_fingerprint = $1`, operatorFingerprint)
		}
	})

	repoDir := t.TempDir()
	writeCanaryFleetRepo(t, repoDir, fleet)
	store := pgstore.NewFromPool(pool)
	if _, err := store.RegisterEndpoint(ctx, endpointID, fleet, "canary-e2e-"+endpointID); err != nil {
		t.Fatal(err)
	}
	caCert, caKey, _ := canaryTestCA(t)
	operator, err := pki.IssueOperatorCredential(caCert, caKey, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	operatorFingerprint = identity.Fingerprint(operator.Cert)
	if err := store.RegisterOperatorCredential(ctx, operatorFingerprint); err != nil {
		t.Fatal(err)
	}

	newServer := func(current *pgstore.Store) *server.Server {
		return server.New(server.Config{
			ConfigRepoPath: repoDir,
			ReleaseRef:     "canary-e2e",
			Registry:       current,
			Telemetry:      current,
			Admin:          &pgstore.RegistryAdmin{Store: current},
			StateReports:   current,
		})
	}
	firstServer := newServer(store)

	agentStateDir := t.TempDir()
	managedPath := filepath.Join(t.TempDir(), "managed.conf")
	artifact := []byte("schemaVersion: 1\nconfigurations:\n  - name: base\n    resources:\n      - kind: file\n        name: managed\n        path: " + managedPath + "\n        content: " + canary + "\n")
	firstSync, firstLogs := runCanaryAgent(t, ctx, artifact, agentStateDir, "digest-canary-first")
	assertCanaryAbsent(t, canary, "first agent logs", []byte(firstLogs))
	assertCanaryAbsent(t, canary, "first Sync payload", firstSync)
	postCanarySync(t, firstServer.Handler(), endpointID, firstSync, canary)

	// Reconstruct both sides from durable state. A fresh engine uses the same
	// endpoint state directory, while the new server/store facade reads only
	// the Postgres rows written by the authenticated first Sync.
	restartedStore := pgstore.NewFromPool(pool)
	restartedServer := newServer(restartedStore)
	secondSync, secondLogs := runCanaryAgent(t, ctx, artifact, agentStateDir, "digest-canary-second")
	assertCanaryAbsent(t, canary, "restarted agent logs", []byte(secondLogs))
	assertCanaryAbsent(t, canary, "restarted agent Sync payload", secondSync)
	postCanarySync(t, restartedServer.Handler(), endpointID, secondSync, canary)

	var persistedReports [][]byte
	rows, err := pool.Query(ctx, `SELECT report_json FROM drift_reports WHERE endpoint_id = $1 ORDER BY reported_at`, endpointID)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var persisted []byte
		if err := rows.Scan(&persisted); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		persistedReports = append(persistedReports, persisted)
		assertCanaryAbsent(t, canary, "Postgres report_json", persisted)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		t.Fatal(err)
	}
	rows.Close()
	if len(persistedReports) != 2 {
		t.Fatalf("persisted report count = %d, want 2 agent process attempts", len(persistedReports))
	}

	apiBody := getCanaryAdminReport(t, restartedServer.Handler(), endpointID, operator.Cert)
	assertCanaryAbsent(t, canary, "restarted Admin API", apiBody)
	cliOutput, fixtureDir := runCanaryCLI(t, endpointID, apiBody)
	assertCanaryAbsent(t, canary, "operator CLI", cliOutput)
	assertNoCanaryBelow(t, fixtureDir, []byte(canary))

	if err := os.WriteFile(filepath.Join(agentStateDir, "state.json"), []byte(`{"token":"`+canary+`"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 18, 12, 0, 0, 0, time.UTC)
	bundle, err := agentdiagnostics.Collect(ctx, agentdiagnostics.Options{
		Spec: diagcatalog.Spec{
			Collectors: []string{diagcatalog.CollectorJournalRemotr, diagcatalog.CollectorRemotrAgentState},
			Since:      now.Add(-time.Hour),
			Until:      now,
		},
		RequestID:    "canary-e2e",
		AgentVersion: "e2e",
		StateDir:     agentStateDir,
		Runner:       canaryDiagnosticRunner{canary: canary},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := diagcatalog.ValidateBundle(bundle.Data); err != nil {
		t.Fatalf("validate diagnostic bundle: %v", err)
	}
	for name, content := range readCanaryDiagnosticArchive(t, bundle.Data) {
		assertCanaryAbsent(t, canary, "diagnostic bundle "+name, content)
	}

	keyring, err := secrets.NewKeyring("kek-canary-e2e", map[string][]byte{
		"kek-canary-e2e": bytes.Repeat([]byte{0xc7}, 32),
	})
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := secrets.NewEnvelope(keyring)
	if err != nil {
		t.Fatal(err)
	}
	secretService, err := secrets.NewRegistryService(restartedStore, envelope, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := secretService.Upload(ctx, secrets.UploadRequest{
		Name: secretName, Fleet: fleet, Material: []byte(canary), ActorID: "canary-e2e",
	})
	if err != nil {
		t.Fatal(err)
	}
	var encryptedDatabaseRecord []byte
	if err := pool.QueryRow(ctx, `SELECT envelope_json FROM secret_versions WHERE name = $1 AND version = 1`, secretName).Scan(&encryptedDatabaseRecord); err != nil {
		t.Fatal(err)
	}
	assertCanaryAbsent(t, canary, "encrypted Postgres backup record", encryptedDatabaseRecord)

	restoredStore := pgstore.NewFromPool(pool)
	restoredRecords, err := restoredStore.ListEncryptedSecretRecords(ctx)
	if err != nil {
		t.Fatal(err)
	}
	restoredJSON, err := json.Marshal(restoredRecords)
	if err != nil {
		t.Fatal(err)
	}
	assertCanaryAbsent(t, canary, "restored encrypted-record inventory", restoredJSON)
	coverage, err := envelope.CheckKeyCoverage(ctx, restoredRecords)
	if err != nil || !coverage.Complete {
		t.Fatalf("restored external KEK coverage = %+v, err=%v", coverage, err)
	}
	restoredService, err := secrets.NewRegistryService(restoredStore, envelope, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := restoredService.Resolve(ctx, secrets.ResolveRequest{
		Reference: "remotr:" + secretName + "@" + metadata.Version,
		Fleet:     fleet, ResourceAddress: "base/managed", Purpose: "e2e-recovery",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(resolved.Material, []byte(canary)) {
		t.Fatalf("restored secret material did not match the protected canary")
	}
	clear(resolved.Material)

	rollbackRoot := filepath.Join(t.TempDir(), "rollback")
	rollbackNow := time.Date(2026, 7, 18, 12, 30, 0, 0, time.UTC)
	rollbackOptions := rollbackstore.Options{
		Root:        rollbackRoot,
		Now:         func() time.Time { return rollbackNow },
		KeyProvider: canaryRollbackKeyProvider{},
	}
	localRollback, err := rollbackstore.New(rollbackOptions)
	if err != nil {
		t.Fatal(err)
	}
	recoveryRecord := rollbackstore.Record{
		Address: "base/managed", ArtifactDigest: "sha256:canary-e2e", Attempt: 1,
		Payload: []byte(canary), Armed: true, Sensitive: true,
		ExpiresAt: rollbackNow.Add(time.Hour),
	}
	if err := localRollback.Save(ctx, recoveryRecord); err != nil {
		t.Fatal(err)
	}
	assertNoCanaryBelow(t, rollbackRoot, []byte(canary))
	restartedRollback, err := rollbackstore.New(rollbackOptions)
	if err != nil {
		t.Fatal(err)
	}
	recoveryCalls := 0
	if err := restartedRollback.RecoverArmed(ctx, func(_ context.Context, recovery rollbackstore.Recovery) error {
		recoveryCalls++
		if recovery.Address != recoveryRecord.Address || recovery.ArtifactDigest != recoveryRecord.ArtifactDigest || recovery.Attempt != recoveryRecord.Attempt || !bytes.Equal(recovery.Payload, []byte(canary)) {
			t.Fatalf("reconstructed rollback recovery = %+v", recovery)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if recoveryCalls != 1 {
		t.Fatalf("rollback recovery callback count = %d, want 1", recoveryCalls)
	}
	assertNoCanaryBelow(t, rollbackRoot, []byte(canary))
	cleanRestart, err := rollbackstore.New(rollbackOptions)
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanRestart.RecoverArmed(ctx, func(context.Context, rollbackstore.Recovery) error {
		t.Fatal("terminal secret-bearing recovery was replayed after cleanup")
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := restoredService.DeleteVersion(ctx, secrets.DeleteVersionRequest{
		Name: secretName, Version: metadata.Version, ActorID: "canary-e2e-cleanup",
	}); err != nil {
		t.Fatal(err)
	}
	var secretRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM secret_versions WHERE name = $1`, secretName).Scan(&secretRows); err != nil {
		t.Fatal(err)
	}
	if secretRows != 0 {
		t.Fatalf("secret version cleanup retained %d protected row(s)", secretRows)
	}
	removed, err := restartedStore.DeleteEndpoint(ctx, endpointID)
	if err != nil || !removed {
		t.Fatalf("endpoint cleanup removed=%t err=%v", removed, err)
	}
	var reportRows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM drift_reports WHERE endpoint_id = $1`, endpointID).Scan(&reportRows); err != nil {
		t.Fatal(err)
	}
	if reportRows != 0 {
		t.Fatalf("endpoint cleanup retained %d state-report row(s)", reportRows)
	}

	t.Logf("secret canary remained absent from agent logs, two Sync payloads, two Postgres reports, restarted Admin API/CLI output, diagnostics, encrypted backup/restore metadata, rollback storage, and terminal persistence; protected recovery callbacks matched once and cleanup left zero secret/report rows")
}

func runCanaryAgent(t *testing.T, ctx context.Context, artifact []byte, stateDir, digest string) ([]byte, string) {
	t.Helper()
	var logs bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	defer slog.SetDefault(previousLogger)
	result, err := pipeline.Run(ctx, artifact, engine.PolicyReport, nil, nil, "https://remotr.example",
		engine.WithStateDir(stateDir), engine.WithArtifactDigest(digest))
	if err != nil {
		t.Fatal(err)
	}
	var pending agentsync.Pending
	pending.SetFromPipeline(result.Labels, result.Drift, result.Apply, result.ApplyFailure, digest)
	body, err := json.Marshal(pending.Request("", "", "v2.0.0"))
	if err != nil {
		t.Fatal(err)
	}
	return body, logs.String()
}

func postCanarySync(t *testing.T, handler http.Handler, endpointID string, body []byte, canary string) {
	t.Helper()
	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Sync status = %d, body = %s", rec.Code, rec.Body.String())
	}
	assertCanaryAbsent(t, canary, "Sync response", rec.Body.Bytes())
}

func getCanaryAdminReport(t *testing.T, handler http.Handler, endpointID string, operator *x509.Certificate) []byte {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/admin/endpoints/"+endpointID+"/state-report", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{operator}}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("Admin API status = %d, body = %s", rec.Code, rec.Body.String())
	}
	return bytes.Clone(rec.Body.Bytes())
}

func runCanaryCLI(t *testing.T, endpointID string, apiBody []byte) ([]byte, string) {
	t.Helper()
	fixtureDir := t.TempDir()
	fixture, err := json.Marshal(struct {
		Status int             `json:"status"`
		Body   json.RawMessage `json:"body"`
	}{Status: http.StatusOK, Body: json.RawMessage(apiBody)})
	if err != nil {
		t.Fatal(err)
	}
	fixturePath := filepath.Join(fixtureDir, "GET_v1_admin_endpoints_"+endpointID+"_state-report.json")
	if err := os.WriteFile(fixturePath, fixture, 0o600); err != nil {
		t.Fatal(err)
	}
	stateDir := filepath.Join(t.TempDir(), "operator")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"operator.crt", "operator.key", "ca.crt", "state.json"} {
		if err := os.WriteFile(filepath.Join(stateDir, name), []byte("test"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	command := remotrCommand(
		t,
		"https://demo.remotr.example",
		filepath.Join(stateDir, "ca.crt"),
		stateDir,
		"endpoint", "state", "report", "--endpoint", endpointID, "--json",
	)
	command.Env = append(os.Environ(), "REMOTR_DEMO=1", "REMOTR_DEMO_FIXTURES="+fixtureDir)
	output, _ := command.CombinedOutput() // Drift intentionally returns exit code 4.
	if len(output) == 0 {
		t.Fatal("operator CLI produced no state report output")
	}
	return output, fixtureDir
}

func writeCanaryFleetRepo(t *testing.T, root, fleet string) {
	t.Helper()
	modulePath := filepath.Join(root, "modules", fleet+"-module.yaml")
	if err := os.MkdirAll(filepath.Dir(modulePath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modulePath, []byte("kind: module\nconfigurations:\n  - name: base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(root, "fleets", fleet, "manifest.yaml")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, []byte("kind: manifest\nmodules:\n  - modules/"+fleet+"-module.yaml\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func canaryTestCA(t *testing.T) (*x509.Certificate, *rsa.PrivateKey, []byte) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Remotr Canary E2E CA"},
		NotBefore: now.Add(-time.Minute), NotAfter: now.AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

type canaryDiagnosticRunner struct{ canary string }

func (runner canaryDiagnosticRunner) Run(context.Context, string, ...string) ([]byte, error) {
	return []byte("diagnostic source " + runner.canary + "\n"), nil
}

type canaryRollbackKeyProvider struct{}

func (canaryRollbackKeyProvider) LoadOrCreate(context.Context, string) (rollbackstore.KeyMaterial, error) {
	return rollbackstore.KeyMaterial{
		ID: "canary-e2e-v1", Key: bytes.Repeat([]byte{0x7c}, 32),
		Protection: rollbackstore.ProtectionRootFile,
	}, nil
}

func readCanaryDiagnosticArchive(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	reader := tar.NewReader(gz)
	files := make(map[string][]byte)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return files
		}
		if err != nil {
			t.Fatal(err)
		}
		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}
		files[header.Name] = content
	}
}

func assertCanaryAbsent(t *testing.T, canary, sink string, content []byte) {
	t.Helper()
	if bytes.Contains(content, []byte(canary)) {
		t.Fatalf("%s leaked secret canary: %s", sink, content)
	}
}

func assertNoCanaryBelow(t *testing.T, root string, canary []byte) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if bytes.Contains(content, canary) {
			return &canaryLeakError{path: path}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type canaryLeakError struct{ path string }

func (err *canaryLeakError) Error() string { return "secret canary persisted in " + err.path }
