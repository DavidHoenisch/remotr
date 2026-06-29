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

	"github.com/DavidHoenisch/remotr/internal/cronscheduler"
	"github.com/DavidHoenisch/remotr/internal/models"
	"github.com/DavidHoenisch/remotr/internal/registry"
	pgstore "github.com/DavidHoenisch/remotr/internal/store/postgres"
)

type mockCronScheduler struct {
	lastRuns map[string]map[string]cronscheduler.LastRun
	dispatched []cronscheduler.DueJob
}

func (m *mockCronScheduler) RecordCronResults(_ context.Context, endpointID, _, _ string, results []pgstore.CronResultPayload) error {
	if m.lastRuns == nil {
		m.lastRuns = map[string]map[string]cronscheduler.LastRun{}
	}
	if m.lastRuns[endpointID] == nil {
		m.lastRuns[endpointID] = map[string]cronscheduler.LastRun{}
	}
	for _, result := range results {
		m.lastRuns[endpointID][result.CronName] = cronscheduler.LastRun{
			CronName:     result.CronName,
			RunID:        result.RunID,
			Status:       result.Status,
			StartedAt:    result.StartedAt,
			CompletedAt:  result.CompletedAt,
			Message:      result.Message,
		}
	}
	return nil
}

func (m *mockCronScheduler) MarkCronRunning(_ context.Context, endpointID, _ string, due cronscheduler.DueJob, startedAt time.Time) error {
	if m.lastRuns == nil {
		m.lastRuns = map[string]map[string]cronscheduler.LastRun{}
	}
	if m.lastRuns[endpointID] == nil {
		m.lastRuns[endpointID] = map[string]cronscheduler.LastRun{}
	}
	m.lastRuns[endpointID][due.CronName] = cronscheduler.LastRun{
		CronName:     due.CronName,
		RunID:        due.RunID,
		ScheduledFor: due.ScheduledFor,
		Status:       "running",
		StartedAt:    startedAt,
	}
	m.dispatched = append(m.dispatched, due)
	return nil
}

func (m *mockCronScheduler) ListCronLastRuns(_ context.Context, endpointID string) (map[string]cronscheduler.LastRun, error) {
	if m.lastRuns == nil || m.lastRuns[endpointID] == nil {
		return map[string]cronscheduler.LastRun{}, nil
	}
	out := make(map[string]cronscheduler.LastRun, len(m.lastRuns[endpointID]))
	for k, v := range m.lastRuns[endpointID] {
		out[k] = v
	}
	return out, nil
}

func (m *mockCronScheduler) GetEndpointCronReport(_ context.Context, endpointID string, jobs []registry.CronJobStatus) (registry.CronReport, bool, error) {
	return registry.CronReport{EndpointID: endpointID, Jobs: jobs}, true, nil
}

func (m *mockCronScheduler) ListFleetCronReports(_ context.Context, fleet string, jobsForEndpoint func(string, map[string]string) []registry.CronJobStatus) (registry.FleetCronReport, error) {
	return registry.FleetCronReport{Fleet: fleet}, nil
}

func TestSync_returnsDueCronsWhenScheduled(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetWithCrons(t, repoDir, "test-fleet", `configurations:
  - name: smoke
    commands:
      - name: noop
        apply: [true]
`, `crons:
  - name: always-due
    schedule: "* * * * *"
    commands:
      - name: run
        apply: [true]
`)

	reg := registry.NewMemory()
	reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"})
	cronMock := &mockCronScheduler{}

	uri, err := url.Parse("urn:remotr:endpoint:" + endpointID)
	if err != nil {
		t.Fatal(err)
	}

	srv := New(Config{
		ConfigRepoPath:  repoDir,
		ReleaseRef:      "e2e",
		Registry:        reg,
		CronScheduler:   cronMock,
	})

	body, _ := json.Marshal(map[string]any{
		"lastDigest": "",
		"labels": map[string]string{
			"distro": "Debian",
			"arch":   "x86",
		},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = &tls.ConnectionState{
		PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}},
	}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}

	var resp syncResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.DueCrons) != 1 {
		t.Fatalf("dueCrons = %+v", resp.DueCrons)
	}
	if resp.DueCrons[0].CronName != "always-due" {
		t.Fatalf("cron = %q", resp.DueCrons[0].CronName)
	}
	if len(resp.DueCrons[0].SpecYAML) == 0 {
		t.Fatal("expected spec yaml")
	}
}

func TestSync_persistsCronResults(t *testing.T) {
	endpointID := "11111111-1111-1111-1111-111111111111"
	repoDir := t.TempDir()
	writeTestFleetWithCrons(t, repoDir, "test-fleet", `configurations:
  - name: smoke
    commands:
      - name: noop
        apply: [true]
`, `crons:
  - name: x
    schedule: "0 0 * * 0"
    commands:
      - name: run
        apply: [true]
`)

	reg := registry.NewMemory()
	reg.RegisterEndpoint(registry.Endpoint{ID: endpointID, Fleet: "test-fleet"})
	cronMock := &mockCronScheduler{}

	uri, _ := url.Parse("urn:remotr:endpoint:" + endpointID)
	srv := New(Config{ConfigRepoPath: repoDir, Registry: reg, CronScheduler: cronMock})

	started := time.Date(2026, 6, 18, 0, 0, 0, 0, time.UTC)
	body, _ := json.Marshal(map[string]any{
		"lastDigest": "",
		"cronResults": []map[string]any{{
			"runId":       "22222222-2222-2222-2222-222222222222",
			"cronName":    "x",
			"status":      "success",
			"startedAt":   started.Format(time.RFC3339),
			"completedAt": started.Add(time.Minute).Format(time.RFC3339),
		}},
	})
	req := httptest.NewRequest(http.MethodPost, "/v1/sync", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{{URIs: []*url.URL{uri}}}}

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}

	last := cronMock.lastRuns[endpointID]["x"]
	if last.Status != "success" {
		t.Fatalf("status = %q", last.Status)
	}
}

var _ models.CronJob
