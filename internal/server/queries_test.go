package server

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/identity"
	"github.com/DavidHoenisch/remotr/internal/pki"
	"github.com/DavidHoenisch/remotr/internal/rbac"
	"github.com/DavidHoenisch/remotr/internal/registry"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
)

func writeTestFleetWithQueries(t *testing.T, repoDir, fleet string) {
	t.Helper()
	mustWrite := func(rel, content string) {
		p := filepath.Join(repoDir, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("modules/base.yaml", `kind: module
configurations:
  - name: base
    commands:
      - name: noop
        apply: [true]
`)
	mustWrite("queries/security.yaml", `kind: queries
schemaVersion: 1
queries:
  - name: listening-ports
    description: Non-loopback listening sockets
    schedule: "*/5 * * * *"
    mode: diff
    retentionRuns: 5
    sql: "SELECT port, process FROM listening_ports"
`)
	mustWrite("fleets/"+fleet+"/manifest.yaml", `kind: manifest
modules:
  - modules/base.yaml
queries:
  - queries/security.yaml
`)
}

// OS-OSQ-007, OS-OSQ-008, OS-OSQ-009, OS-OSQ-010: authenticated sync dispatches
// scheduled and ad-hoc queries, ingests snapshots and server-side diffs, and
// enforces RBAC separation between query:read, query:run, and query:run_sql.
func TestQueries_EndToEndSyncDiffAdhocAndRBAC(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetWithQueries(t, repoDir, "prod")

	reg := registry.NewMemory()
	uri, err := url.Parse("urn:remotr:endpoint:" + endpointID)
	if err != nil {
		t.Fatal(err)
	}
	cert := &x509.Certificate{
		Raw:  []byte("agent-cert-1"),
		URIs: []*url.URL{uri},
	}
	fp := identity.Fingerprint(cert)
	if err := reg.RegisterEndpoint(registry.Endpoint{
		ID:              endpointID,
		Fleet:           "prod",
		CertFingerprint: fp,
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 11, 19, 10, 32, 0, 0, time.UTC)
	srv := New(Config{
		ConfigRepoPath: repoDir,
		Registry:       reg,
		Admin:          reg,
		Now:            func() time.Time { return now },
	})
	handler := srv.Handler()

	agentPost := func(path string, body any) *httptest.ResponseRecorder {
		raw, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{cert}}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	// 1. First sync dispatches scheduled query `listening-ports`.
	rr := agentPost("/v1/sync", syncRequest{})
	if rr.Code != http.StatusOK {
		t.Fatalf("first sync status = %d: %s", rr.Code, rr.Body.String())
	}
	var resp syncResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.DueQueries) != 1 || resp.DueQueries[0].Name != "listening-ports" {
		t.Fatalf("DueQueries = %+v (resp=%+v), want single listening-ports", resp.DueQueries, resp)
	}
	firstRunID := resp.DueQueries[0].RunID

	// 2. Agent posts initial snapshot results via /v1/sync/query-results.
	rr = agentPost("/v1/sync/query-results", map[string]any{
		"results": []queryResultPayload{{
			RunID:      firstRunID,
			Name:       "listening-ports",
			Status:     "success",
			Rows:       []map[string]string{{"port": "22", "process": "sshd"}, {"port": "80", "process": "nginx"}},
			RowCount:   2,
			DurationMS: 42,
		}},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("post query-results status = %d: %s", rr.Code, rr.Body.String())
	}

	// 3. Advance clock to next 5-minute slot, sync again, report changed rows -> diff computed!
	now = now.Add(5 * time.Minute)
	rr = agentPost("/v1/sync", syncRequest{LastDigest: resp.Digest})
	if rr.Code != http.StatusOK {
		t.Fatalf("second sync status = %d: %s", rr.Code, rr.Body.String())
	}
	var resp2 syncResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp2); err != nil {
		t.Fatal(err)
	}
	if len(resp2.DueQueries) != 1 {
		t.Fatalf("second sync DueQueries = %+v, want 1", resp2.DueQueries)
	}
	secondRunID := resp2.DueQueries[0].RunID

	rr = agentPost("/v1/sync/query-results", map[string]any{
		"results": []queryResultPayload{{
			RunID:      secondRunID,
			Name:       "listening-ports",
			Status:     "success",
			Rows:       []map[string]string{{"port": "22", "process": "sshd"}, {"port": "443", "process": "nginx"}},
			RowCount:   2,
			DurationMS: 38,
		}},
	})
	if rr.Code != http.StatusOK {
		t.Fatalf("second post query-results status = %d: %s", rr.Code, rr.Body.String())
	}

	// Verify server-side diff persisted added=443, removed=80.
	diffs, err := srv.cfg.QueryStore.ListQueryDiffs(t.Context(), "listening-ports", "prod", endpointID, time.Time{}, 10)
	if err != nil || len(diffs) < 2 {
		t.Fatalf("ListQueryDiffs = %+v, err=%v", diffs, err)
	}
	latestDiff := diffs[0]
	if latestDiff.AddedCount != 1 || latestDiff.AddedRows[0]["port"] != "443" || latestDiff.RemovedCount != 1 || latestDiff.RemovedRows[0]["port"] != "80" {
		t.Fatalf("latestDiff = %+v, want +443 / -80", latestDiff)
	}

	// 4. Verify HTTP RBAC separation: query_runner can trigger named runs
	// but receives HTTP 403 on inline run-sql (OS-OSQ-010).
	caCert, caKey, caPEM := testCAForEnroll(t)
	runnerID := "22222222-2222-2222-2222-222222222222"
	adminID := "33333333-3333-3333-3333-333333333333"
	runnerCred, err := pki.IssueOperatorCredential(caCert, caKey, runnerID)
	if err != nil {
		t.Fatal(err)
	}
	adminCred, err := pki.IssueOperatorCredential(caCert, caKey, adminID)
	if err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterOperator(runnerID, identity.Fingerprint(runnerCred.Cert), []string{rbac.RoleQueryRunner}); err != nil {
		t.Fatal(err)
	}
	if err := reg.RegisterOperator(adminID, identity.Fingerprint(adminCred.Cert), []string{rbac.RoleGlobalAdmin}); err != nil {
		t.Fatal(err)
	}
	srv.cfg.CACert, srv.cfg.CAKey, srv.cfg.CACertPEM = caCert, caKey, caPEM
	srv.cfg.RBAC = &mockRBAC{
		roles: map[string][]string{
			runnerID: {rbac.RoleQueryRunner},
			adminID:  {rbac.RoleGlobalAdmin},
		},
	}
	opCert := adminCred.Cert
	runnerReq := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{runnerCred.Cert}}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}
	forbiddenSQL := runnerReq(http.MethodPost, "/v1/admin/queries/run-sql", queryRunRequest{
		EndpointID: endpointID,
		SQL:        "SELECT 1",
	})
	if forbiddenSQL.Code != http.StatusForbidden {
		t.Fatalf("query_runner POST /v1/admin/queries/run-sql status = %d, want 403", forbiddenSQL.Code)
	}
	readerRole, _ := rbac.BuiltInRole(rbac.RoleQueryReader)
	runnerRole, _ := rbac.BuiltInRole(rbac.RoleQueryRunner)
	sqlRole, _ := rbac.BuiltInRole(rbac.RoleQuerySQLRunner)
	if !rbac.Allow(readerRole.Rules, "GET", "/v1/admin/queries/listening-ports/results") || rbac.Allow(readerRole.Rules, "POST", "/v1/admin/queries/run") {
		t.Fatal("query_reader RBAC rules misconfigured")
	}
	if !rbac.Allow(runnerRole.Rules, "POST", "/v1/admin/queries/run") || rbac.Allow(runnerRole.Rules, "POST", "/v1/admin/queries/run-sql") {
		t.Fatal("query_runner must allow named run but deny inline run-sql")
	}
	if !rbac.Allow(sqlRole.Rules, "POST", "/v1/admin/queries/run-sql") {
		t.Fatal("query_sql_runner must allow inline run-sql")
	}

	// 5. Ad-hoc named run + inline SQL validation + agent pickup + completion.
	opReq := func(method, path string, body any) *httptest.ResponseRecorder {
		var buf bytes.Buffer
		if body != nil {
			_ = json.NewEncoder(&buf).Encode(body)
		}
		req := httptest.NewRequest(method, path, &buf)
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{opCert}}
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr
	}

	// Reject mutating inline SQL with 400.
	badSQL := opReq(http.MethodPost, "/v1/admin/queries/run-sql", queryRunRequest{
		EndpointID: endpointID,
		SQL:        "DROP TABLE users",
	})
	if badSQL.Code != http.StatusBadRequest {
		t.Fatalf("bad inline SQL status = %d, want 400", badSQL.Code)
	}

	// Trigger valid ad-hoc named query.
	runRR := opReq(http.MethodPost, "/v1/admin/queries/run", queryRunRequest{
		QueryName:  "listening-ports",
		EndpointID: endpointID,
	})
	if runRR.Code != http.StatusOK {
		t.Fatalf("ad-hoc run status = %d: %s", runRR.Code, runRR.Body.String())
	}
	var created adhocRunResponse
	if err := json.NewDecoder(runRR.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}
	if created.Request.RequestID == "" || created.ExpectedTargets != 1 {
		t.Fatalf("created adhoc run = %+v", created)
	}

	// Endpoint sync picks up pending ad-hoc query.
	syncRR := agentPost("/v1/sync", syncRequest{LastDigest: resp.Digest})
	var syncResp3 syncResponse
	if err := json.NewDecoder(syncRR.Body).Decode(&syncResp3); err != nil {
		t.Fatal(err)
	}
	if len(syncResp3.AdhocQueries) != 1 || syncResp3.AdhocQueries[0].RequestID != created.Request.RequestID {
		t.Fatalf("AdhocQueries = %+v, want request %s", syncResp3.AdhocQueries, created.Request.RequestID)
	}

	// Endpoint reports ad-hoc result back.
	_ = agentPost("/v1/sync/query-results", map[string]any{
		"results": []queryResultPayload{{
			RunID:       syncResp3.AdhocQueries[0].RunID,
			RequestID:   created.Request.RequestID,
			TriggerType: "adhoc",
			Name:        "listening-ports",
			Status:      "success",
			Rows:        []map[string]string{{"port": "443", "process": "nginx"}},
			RowCount:    1,
		}},
	})

	// Operator polls ad-hoc run and gets results.
	pollRR := opReq(http.MethodGet, "/v1/admin/queries/runs/"+created.Request.RequestID, nil)
	if pollRR.Code != http.StatusOK {
		t.Fatalf("poll adhoc run status = %d: %s", pollRR.Code, pollRR.Body.String())
	}
	var polled adhocRunResponse
	if err := json.NewDecoder(pollRR.Body).Decode(&polled); err != nil {
		t.Fatal(err)
	}
	if len(polled.Results) != 1 || polled.Results[0].Status != "success" || polled.Results[0].Rows[0]["port"] != "443" {
		t.Fatalf("polled adhoc results = %+v", polled.Results)
	}

	// Operator lists fleet queries and reads snapshot results.
	listRR := opReq(http.MethodGet, "/v1/admin/queries?fleet=prod", nil)
	if listRR.Code != http.StatusOK || !strings.Contains(listRR.Body.String(), "listening-ports") {
		t.Fatalf("list queries status=%d body=%s", listRR.Code, listRR.Body.String())
	}
	resRR := opReq(http.MethodGet, "/v1/admin/queries/listening-ports/results?fleet=prod", nil)
	if resRR.Code != http.StatusOK {
		t.Fatalf("results status=%d body=%s", resRR.Code, resRR.Body.String())
	}
	var resBody struct {
		Snapshots []pgstore.QuerySnapshotRecord `json:"snapshots"`
	}
	if err := json.NewDecoder(resRR.Body).Decode(&resBody); err != nil {
		t.Fatal(err)
	}
	if len(resBody.Snapshots) != 1 || resBody.Snapshots[0].RowCount != 2 {
		t.Fatalf("snapshots = %+v", resBody.Snapshots)
	}
}
