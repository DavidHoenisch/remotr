package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DavidHoenisch/remotr/internal/admin"
)

func TestReadExportParityUsesBoundedTypedEvidenceAndNativeDestinations(t *testing.T) {
	app, savedDirectory, dialogRequests := newReadExportParityTestApp(t)

	inventory, err := app.LoadAssetInventory()
	if err != nil {
		t.Fatalf("load asset inventory: %v", err)
	}
	wantInventory := []AssetInventoryRow{{
		EndpointID:     "endpoint-alpha",
		Fleet:          "production",
		OS:             "Debian 13",
		CPU:            "AMD EPYC (4 cores)",
		RAM:            "8 GiB",
		Kernel:         "6.12.0",
		PrimaryIP:      "192.0.2.10",
		MACAddress:     "02:00:00:00:00:10",
		DiskEncryption: "1/1",
		TPM:            "present (version 2.0)",
		AgentVersion:   "v2.1.0",
		LastCheckIn:    "2026-03-04T05:04:07Z",
	}}
	if !slices.Equal(inventory.Rows, wantInventory) || inventory.Section.State != SectionReady {
		t.Fatalf("asset inventory = %#v, want ready %#v", inventory, wantInventory)
	}

	inventorySave, err := app.SaveAssetInventory("json")
	if err != nil {
		t.Fatalf("save asset inventory: %v", err)
	}
	assertReadExportSave(t, inventorySave, filepath.Join(savedDirectory, "remotr-inventory.json"), "endpoint-alpha")

	fleet, err := app.LoadFleetOperationalReports("production")
	if err != nil {
		t.Fatalf("load Fleet operational reports: %v", err)
	}
	if fleet.Fleet != "production" || len(fleet.States) != 1 || fleet.States[0].EndpointID != "endpoint-alpha" {
		t.Fatalf("Fleet State evidence = %#v, want exact Endpoint State", fleet.States)
	}
	if len(fleet.Schedules) != 1 || fleet.Schedules[0].EndpointID != "endpoint-alpha" || fleet.Schedules[0].Name != "daily-audit" {
		t.Fatalf("Fleet schedule evidence = %#v, want exact daily-audit row", fleet.Schedules)
	}
	if fleet.Sections.State.State != SectionReady || fleet.Sections.Schedules.State != SectionReady {
		t.Fatalf("Fleet report sections = %#v, want ready State and schedules", fleet.Sections)
	}

	firewall, err := app.LoadFirewallReport("endpoint-alpha")
	if err != nil {
		t.Fatalf("load Firewall report: %v", err)
	}
	if firewall.EndpointID != "endpoint-alpha" || len(firewall.Audit) != 1 || firewall.Audit[0].RuleName != "allow-ssh" {
		t.Fatalf("Firewall audit evidence = %#v, want exact allow-ssh row", firewall.Audit)
	}
	if firewall.Live.Backend != "firewalld" || firewall.Live.DefaultZone != "public" || len(firewall.Live.Zones) != 1 {
		t.Fatalf("live Firewall evidence = %#v, want firewalld public zone", firewall.Live)
	}
	firewallSave, err := app.SaveFirewallReport(FirewallExportRequest{EndpointID: "endpoint-alpha", Format: "csv"})
	if err != nil {
		t.Fatalf("save Firewall report: %v", err)
	}
	assertReadExportSave(t, firewallSave, filepath.Join(savedDirectory, "endpoint-alpha-firewall.csv"), "endpoint_id,backend,zone,target,service,port,source,raw_ruleset")

	auditInfo, err := app.LoadAuditExportInfo()
	if err != nil {
		t.Fatalf("load audit export info: %v", err)
	}
	if auditInfo.ExportPath != "/v1/admin/audit-export/events" || auditInfo.PathKey != "siem-v1" {
		t.Fatalf("audit export info = %#v, want exact server metadata", auditInfo)
	}

	diagnostic, err := app.LoadDiagnosticRequest("diagnostic-42")
	if err != nil {
		t.Fatalf("load diagnostic lifecycle: %v", err)
	}
	if diagnostic.RequestID != "diagnostic-42" || diagnostic.EndpointID != "endpoint-alpha" || diagnostic.Status != "ready" || diagnostic.SizeBytes != 128 {
		t.Fatalf("diagnostic lifecycle = %#v, want exact ready evidence", diagnostic)
	}
	if diagnostic.CreatedAt != "2026-03-04T05:05:08Z" || diagnostic.CompletedAt != "2026-03-04T05:06:08Z" || diagnostic.ExpiresAt != "2026-03-05T05:05:08Z" {
		t.Fatalf("diagnostic lifecycle timestamps = %#v, want server values", diagnostic)
	}

	dialogRequests.mu.Lock()
	defer dialogRequests.mu.Unlock()
	wantDialogs := []string{"remotr-inventory.json", "endpoint-alpha-firewall.csv"}
	if !slices.Equal(dialogRequests.names, wantDialogs) {
		t.Fatalf("native save dialog suggestions = %v, want %v", dialogRequests.names, wantDialogs)
	}
}

func TestReadExportParityRejectsUnsafeFormatsAndHonorsSaveCancellation(t *testing.T) {
	app, savedDirectory, dialogRequests := newReadExportParityTestApp(t)

	for _, format := range []string{"", "yaml", "../csv"} {
		t.Run(format, func(t *testing.T) {
			if _, err := app.SaveAssetInventory(format); err == nil {
				t.Fatal("unsafe asset inventory format was accepted")
			}
			if _, err := app.SaveFirewallReport(FirewallExportRequest{EndpointID: "endpoint-alpha", Format: format}); err == nil {
				t.Fatal("unsafe Firewall format was accepted")
			}
		})
	}
	dialogRequests.mu.Lock()
	if len(dialogRequests.names) != 0 {
		t.Fatalf("unsafe formats reached native save dialog: %v", dialogRequests.names)
	}
	dialogRequests.mu.Unlock()

	app.readExport = NewReadExportService(func(context.Context, ReadExportDialogRequest) (string, error) {
		return "", nil
	})
	result, err := app.SaveAssetInventory("csv")
	if err != nil {
		t.Fatalf("cancel native save: %v", err)
	}
	if result != (ReadExportSaveResult{Status: "canceled"}) {
		t.Fatalf("canceled save result = %#v", result)
	}
	entries, err := os.ReadDir(savedDirectory)
	if err != nil {
		t.Fatalf("read save directory: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("canceled save wrote files: %v", entries)
	}
}

func TestReadExportSystemPayloadsAreBounded(t *testing.T) {
	largeText := strings.Repeat("x", readExportTextLimit+128)
	largeList := make([]string, readExportListLimit+1)
	for index := range largeList {
		largeList[index] = "value"
	}
	zones := make([]map[string]any, readExportZoneLimit+1)
	for index := range zones {
		zones[index] = map[string]any{
			"name": "zone", "target": "default",
		}
	}
	zones[0] = map[string]any{
		"name": largeText, "target": largeText, "services": largeList,
		"ports": largeList, "sources": largeList, "richRules": largeList,
	}
	report, err := json.Marshal(map[string]any{
		"osRelease": map[string]any{"prettyName": largeText},
		"firewall": map[string]any{
			"backend":   "firewalld",
			"firewalld": map[string]any{"defaultZone": largeText, "zones": zones},
		},
	})
	if err != nil {
		t.Fatalf("encode boundary report: %v", err)
	}
	system := &admin.SystemInfoSummary{Report: report}
	row := mapAssetInventoryRow(admin.Endpoint{ID: "endpoint-alpha", SystemInfo: system})
	if len(row.OS) != readExportTextLimit {
		t.Fatalf("asset text boundary = %d, want %d", len(row.OS), readExportTextLimit)
	}
	live, err := mapFirewallLiveEvidence(system)
	if err != nil {
		t.Fatalf("map list boundary report: %v", err)
	}
	if len(live.Zones) != readExportZoneLimit || len(live.Zones[0].Services) != readExportListLimit || len(live.Zones[0].Name) != readExportTextLimit {
		t.Fatalf("Firewall list boundary = zones:%d services:%d name:%d", len(live.Zones), len(live.Zones[0].Services), len(live.Zones[0].Name))
	}

	oversized := &admin.SystemInfoSummary{Report: json.RawMessage(strings.Repeat(" ", readExportSystemReportLimit+1))}
	if row := mapAssetInventoryRow(admin.Endpoint{ID: "endpoint-alpha", SystemInfo: oversized}); row.OS != "" {
		t.Fatalf("oversized asset report was parsed: %#v", row)
	}
	if _, err := mapFirewallLiveEvidence(oversized); err == nil {
		t.Fatal("oversized live Firewall report was accepted")
	}

	boundedReport, err := json.Marshal(map[string]any{
		"firewall": map[string]any{
			"backend":  "nftables",
			"nftables": map[string]any{"rawRuleset": strings.Repeat("r", readExportRulesetLimit+1)},
		},
	})
	if err != nil {
		t.Fatalf("encode ruleset boundary report: %v", err)
	}
	live, err = mapFirewallLiveEvidence(&admin.SystemInfoSummary{Report: boundedReport})
	if err != nil {
		t.Fatalf("map bounded live Firewall report: %v", err)
	}
	if len(live.Ruleset) != readExportRulesetLimit || !live.RulesetTruncated {
		t.Fatalf("ruleset boundary = %d/%t, want %d/true", len(live.Ruleset), live.RulesetTruncated, readExportRulesetLimit)
	}
}

func FuzzReadExportSystemPayloadsRemainBounded(f *testing.F) {
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"osRelease":{"prettyName":"Debian 13"},"firewall":{"backend":"firewalld","firewalld":{"defaultZone":"public","zones":[]}}}`))
	f.Fuzz(func(t *testing.T, report []byte) {
		system := &admin.SystemInfoSummary{Report: report}
		row := mapAssetInventoryRow(admin.Endpoint{ID: "endpoint-alpha", SystemInfo: system})
		for name, value := range map[string]string{
			"agent": row.AgentVersion, "cpu": row.CPU, "fleet": row.Fleet,
			"kernel": row.Kernel, "mac": row.MACAddress, "os": row.OS,
			"primary_ip": row.PrimaryIP, "ram": row.RAM, "tpm": row.TPM,
		} {
			if len(value) > readExportTextLimit {
				t.Fatalf("%s length = %d, want <= %d", name, len(value), readExportTextLimit)
			}
		}
		live, _ := mapFirewallLiveEvidence(system)
		if len(live.Ruleset) > readExportRulesetLimit || len(live.Zones) > readExportZoneLimit {
			t.Fatalf("live Firewall bounds exceeded: ruleset=%d zones=%d", len(live.Ruleset), len(live.Zones))
		}
		for _, zone := range live.Zones {
			for _, values := range [][]string{zone.Services, zone.Ports, zone.Sources, zone.RichRules} {
				if len(values) > readExportListLimit {
					t.Fatalf("Firewall list length = %d, want <= %d", len(values), readExportListLimit)
				}
				for _, value := range values {
					if len(value) > readExportTextLimit {
						t.Fatalf("Firewall value length = %d, want <= %d", len(value), readExportTextLimit)
					}
				}
			}
		}
	})
}

type readExportDialogRequests struct {
	mu    sync.Mutex
	names []string
}

func assertReadExportSave(t *testing.T, result ReadExportSaveResult, wantPath, contains string) {
	t.Helper()
	if result.Status != "saved" || result.Path != wantPath || result.SizeBytes <= 0 {
		t.Fatalf("save result = %#v, want saved metadata for %s", result, wantPath)
	}
	content, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("read native export: %v", err)
	}
	if !strings.Contains(string(content), contains) {
		t.Fatalf("native export %q does not contain %q", content, contains)
	}
	info, err := os.Stat(wantPath)
	if err != nil {
		t.Fatalf("stat native export: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("native export mode = %o, want 600", info.Mode().Perm())
	}
}

func newReadExportParityTestApp(t *testing.T) (*App, string, *readExportDialogRequests) {
	t.Helper()
	fixture := newConnectionTLSFixture(t)
	savedDirectory := t.TempDir()
	dialogRequests := &readExportDialogRequests{}

	server := httptest.NewUnstartedServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.TLS == nil || len(request.TLS.PeerCertificates) != 1 {
			http.Error(response, "verified Operator certificate required", http.StatusUnauthorized)
			return
		}
		response.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/v1/admin/me":
			_, _ = response.Write([]byte(`{"operator_id":"operator-read-export","roles":["global_admin"]}`))
		case "/v1/admin/endpoints":
			_, _ = response.Write([]byte(`[{"id":"endpoint-alpha","fleet":"production"}]`))
		case "/v1/admin/endpoints/endpoint-alpha":
			_, _ = response.Write([]byte(`{
				"id":"endpoint-alpha","fleet":"production","reported_agent_version":"v2.1.0",
				"last_check_in":{"release_ref":"release-42","digest":"digest-alpha","at":"2026-03-04T05:04:07Z"},
				"system_info":{"digest":"system-alpha","reported_at":"2026-03-04T05:04:07Z","report":{
					"osRelease":{"prettyName":"Debian 13"},"cpu":{"modelName":"AMD EPYC","coreCount":"4"},
					"ram":{"memTotal":"8 GiB"},"kernel":{"version":"6.12.0"},"tpm":{"version":"2.0"},
					"networks":[{"name":"eth0","macAddress":"02:00:00:00:00:10","ipv4":["192.0.2.10"]}],
					"blockDevices":[{"name":"sda","encrypted":true,"encryptionType":"luks2"}],
					"firewall":{"backend":"firewalld","firewalld":{"defaultZone":"public","zones":[{"name":"public","target":"default","services":["ssh"],"ports":["8443/tcp"],"sources":["192.0.2.0/24"]}]}}
				}}
			}`))
		case "/v1/admin/endpoints/endpoint-alpha/firewall-audit":
			_ = json.NewEncoder(response).Encode(map[string]any{
				"endpoint_id": "endpoint-alpha",
				"reported_at": "2026-03-04T05:04:07Z",
				"report": []map[string]any{{
					"timestamp": "2026-03-04T05:03:07Z", "ruleName": "allow-ssh", "action": "allow", "protocol": "tcp",
					"ports": []int{22}, "sources": []string{"192.0.2.0/24"}, "backend": "nftables", "wouldHave": "accept", "enforced": true,
				}},
			})
		case "/v1/admin/fleets/production/state-report":
			_, _ = response.Write([]byte(`{"fleet":"production","summary":{"total":1,"compliant":1},"endpoints":[{"endpoint_id":"endpoint-alpha","fleet":"production","release_ref":"release-42","digest":"digest-alpha","reported_at":"2026-03-04T05:04:07Z","in_compliance":true,"status":"compliant","items":[{"address":"base/packages","name":"Packages","provider":"packages","status":"compliant","desiredSummary":{"fields":[{"path":"package","sensitivity":"public","projection":"value","text":"curl installed"}]},"observedSummary":{"fields":[{"path":"package","sensitivity":"public","projection":"value","text":"curl 8.7.1"}]}}]}]}`))
		case "/v1/admin/fleets/production/cron-report":
			_, _ = response.Write([]byte(`{"fleet":"production","summary":{"total":1,"applicable":1,"success":1},"endpoints":[{"endpoint_id":"endpoint-alpha","fleet":"production","crons_digest":"cron-alpha","jobs":[{"name":"daily-audit","schedule":"0 2 * * *","applicable":true,"last_scheduled_for":"2026-03-04T02:00:00Z","last_status":"success","last_message":"completed","last_completed_at":"2026-03-04T02:00:05Z"}]}]}`))
		case "/v1/admin/audit-export":
			_, _ = response.Write([]byte(`{"export_path":"/v1/admin/audit-export/events","path_key":"siem-v1"}`))
		case "/v1/admin/diagnostics/diagnostic-42":
			_, _ = response.Write([]byte(`{"id":"diagnostic-42","endpoint_id":"endpoint-alpha","requested_by":"operator-read-export","status":"ready","spec":{"collectors":["system_info"],"since":"2026-03-03T05:05:07Z","until":"2026-03-04T05:05:07Z"},"sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","size_bytes":128,"created_at":"2026-03-04T05:05:08Z","completed_at":"2026-03-04T05:06:08Z","expires_at":"2026-03-05T05:05:08Z"}`))
		default:
			http.NotFound(response, request)
		}
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

	stateDir := fixture.saveClientState(t, "operator-read-export", connectionTestTime.Add(-time.Hour), connectionTestTime.Add(time.Hour), fixture.caPEM)
	manager := NewSessionManager(NewConnectionService().ConnectSession)
	profile := connectionProfileForServer(t, "Production", server.URL, stateDir)
	if err := manager.SwitchProfile(t.Context(), profile); err != nil {
		t.Fatalf("connect read/export Operator: %v", err)
	}
	dialog := func(_ context.Context, request ReadExportDialogRequest) (string, error) {
		dialogRequests.mu.Lock()
		dialogRequests.names = append(dialogRequests.names, request.SuggestedName)
		dialogRequests.mu.Unlock()
		return filepath.Join(savedDirectory, request.SuggestedName), nil
	}
	app := NewApp("test", WithReadExportSaveDialog(dialog))
	app.sessions = manager
	return app, savedDirectory, dialogRequests
}
